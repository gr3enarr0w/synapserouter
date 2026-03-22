package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Defaults (overridden by env vars)
const (
	DefaultIntervalsICUBase = "https://intervals.icu/api/v1"
	DefaultSyncDays        = 14
	ConversationalPace     = 785 // sec/mi (13:05/mi)
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	RunnaICalURL     string
	IntervalsICUBase string
	AthleteID        string
	APIKey           string
	APIToken         string
	SyncDays         int
	WeightKg         float64
	TargetFiveKMin   float64 // 5K target time in minutes
}

func loadConfig() Config {
	c := Config{
		RunnaICalURL:     envOrDefault("RUNNA_ICAL_URL", ""),
		IntervalsICUBase: envOrDefault("INTERVALS_BASE_URL", DefaultIntervalsICUBase),
		AthleteID:        envOrDefault("INTERVALS_ATHLETE_ID", ""),
		APIKey:           "API_KEY", // Intervals.icu always uses "API_KEY" as username
		APIToken:         envOrDefault("INTERVALS_API_TOKEN", ""),
		SyncDays:         DefaultSyncDays,
		WeightKg:         72.6,
		TargetFiveKMin:   35.0,
	}
	if v := os.Getenv("SYNC_DAYS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			c.SyncDays = d
		}
	}
	if v := os.Getenv("ATHLETE_WEIGHT_KG"); v != "" {
		if w, err := strconv.ParseFloat(v, 64); err == nil && w > 0 {
			c.WeightKg = w
		}
	}
	if v := os.Getenv("TARGET_5K_MINUTES"); v != "" {
		if t, err := strconv.ParseFloat(v, 64); err == nil && t > 0 {
			c.TargetFiveKMin = t
		}
	}
	return c
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Runtime state (set at startup from config)
var (
	cfg             Config
	athleteWeightKg float64
	runFTP          float64
	// Convenience aliases from config (used throughout functions)
	IntervalsICUBase string
	AthleteID        string
	APIKey           string
	APIToken         string
	RunnaICalURL     string
)

// Pre-compiled regexes (compiled once at package init, not per-call)
var (
	reReps        = regexp.MustCompile(`(\d+)\s+reps\s+of:`)
	reRestSec     = regexp.MustCompile(`(\d+)s?\s+(?:walking\s+)?rest`)
	reInlineRest  = regexp.MustCompile(`,\s*(\d+)s?\s+(?:walking\s+)?rest`)
	reDistance     = regexp.MustCompile(`(\d+\.?\d*)\s*mi`)
	rePaceAt      = regexp.MustCompile(`at\s+(\d+:\d+)/mi`)
	rePaceLimit   = regexp.MustCompile(`(?:no faster than|at)\s+(\d+:\d+)/mi`)
	reUID         = regexp.MustCompile(`UID:([^\r\n]+)`)
	reSummary     = regexp.MustCompile(`SUMMARY:([^\r\n]+)`)
	reDescription = regexp.MustCompile(`DESCRIPTION:([^\r\n]+(?:\r?\n[ \t][^\r\n]+)*)`)
	reDTStart     = regexp.MustCompile(`DTSTART(?:;[^:]*)?:([^\r\n]+)`)
	reDTEnd       = regexp.MustCompile(`DTEND(?:;[^:]*)?:([^\r\n]+)`)
)

// iCalEvent represents a parsed iCal event
type iCalEvent struct {
	UID         string
	Summary     string
	Description string
	StartDate   time.Time
	EndDate     time.Time
}

// WorkoutStep represents a single step in a workout
type WorkoutStep struct {
	Type        string  // warmup, steady, interval, cooldown, rest
	Duration    int     // seconds
	Distance    float64 // miles
	PaceSecPerMile int  // pace in seconds per mile
	PowerLow    float64 // for ranges
	PowerHigh   float64
	Reps        int     // for intervals
	OnDuration  int     // for intervals
	OffDuration int     // for intervals
	OnPower     float64 // for intervals
	OffPower    float64 // for intervals
}

// parsePace converts "MM:SS/mi" to seconds per mile
func parsePace(paceStr string) int {
	paceStr = strings.TrimSpace(paceStr)
	paceStr = strings.TrimSuffix(paceStr, "/mi")
	
	parts := strings.Split(paceStr, ":")
	if len(parts) != 2 {
		return ConversationalPace
	}
	
	mins, err := strconv.Atoi(parts[0])
	if err != nil {
		return ConversationalPace
	}
	
	secs, err := strconv.Atoi(parts[1])
	if err != nil {
		return ConversationalPace
	}
	
	return mins*60 + secs
}

// parseDistance extracts distance from strings like "1mi", "0.25mi", "3.25mi"
func parseDistance(distStr string) float64 {
	distStr = strings.TrimSpace(distStr)
	distStr = strings.TrimSuffix(distStr, "mi")
	
	dist, err := strconv.ParseFloat(distStr, 64)
	if err != nil {
		return 0
	}
	return dist
}

// paceToPower converts pace to power percentage based on zones
func paceToPower(paceSecPerMile int) float64 {
	if paceSecPerMile >= 785 { // 13:05+/mi - easy/recovery
		return 0.6
	}
	if paceSecPerMile >= 715 { // 11:55-13:05/mi - steady/moderate
		return 0.7
	}
	if paceSecPerMile >= 695 { // 11:35-11:55/mi - tempo
		return 0.8
	}
	if paceSecPerMile >= 630 { // 10:30-11:35/mi - threshold
		return 0.95
	}
	return 1.0 // <10:30/mi - intervals/fast
}

// parseWorkoutDescription parses Runna workout description into steps
// Handles multiple interval blocks like "Fast 6-4-2s"
func parseWorkoutDescription(desc string) []WorkoutStep {
	var steps []WorkoutStep
	
	lines := strings.Split(desc, "\n")
	
	// State tracking
	inIntervalBlock := false
	currentBlock := IntervalBlock{}
	blockSteps := []WorkoutStep{}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Check for interval block start: "X reps of:"
		repsMatch := reReps.FindStringSubmatch(line)
		if len(repsMatch) > 1 {
			// Save previous interval block if exists
			if inIntervalBlock && currentBlock.Reps > 0 && len(blockSteps) > 0 {
				onStep := blockSteps[0]
				offStep := WorkoutStep{Duration: 60, PowerLow: 0.4} // default rest
				if len(blockSteps) > 1 {
					offStep = blockSteps[1]
				}
				steps = append(steps, WorkoutStep{
					Type:        "interval",
					Reps:        currentBlock.Reps,
					OnDuration:  onStep.Duration,
					OffDuration: offStep.Duration,
					OnPower:     onStep.PowerLow,
					OffPower:    offStep.PowerLow,
				})
			}
			
			// Start new block
			inIntervalBlock = true
			currentBlock = IntervalBlock{Reps: 0}
			blockSteps = []WorkoutStep{}
			currentBlock.Reps, _ = strconv.Atoi(repsMatch[1])
			continue
		}
		
		// Check for standalone rest line between interval blocks: "60s walking rest"
		if inIntervalBlock && strings.Contains(strings.ToLower(line), "walking rest") && !strings.HasPrefix(line, "•") {
			restMatch := reRestSec.FindStringSubmatch(line)
			if len(restMatch) > 1 {
				duration, _ := strconv.Atoi(restMatch[1])
				// This ends the current interval block
				if currentBlock.Reps > 0 && len(blockSteps) > 0 {
					onStep := blockSteps[0]
					offStep := WorkoutStep{Duration: 60, PowerLow: 0.4}
					if len(blockSteps) > 1 {
						offStep = blockSteps[1]
					}
					steps = append(steps, WorkoutStep{
						Type:        "interval",
						Reps:        currentBlock.Reps,
						OnDuration:  onStep.Duration,
						OffDuration: offStep.Duration,
						OnPower:     onStep.PowerLow,
						OffPower:    offStep.PowerLow,
					})
				}
				inIntervalBlock = false
				currentBlock = IntervalBlock{}
				blockSteps = []WorkoutStep{}
				// Add the rest step
				steps = append(steps, WorkoutStep{Type: "rest", Duration: duration})
				continue
			}
		}
		
		// Parse warmup line
		if strings.Contains(strings.ToLower(line), "warm up") || strings.Contains(strings.ToLower(line), "warmup") {
			step := parseWarmupLine(line)
			if inIntervalBlock {
				blockSteps = append(blockSteps, step)
			} else {
				steps = append(steps, step)
			}
			continue
		}
		
		// Parse cooldown line - this also ENDS any active interval block
		if strings.Contains(strings.ToLower(line), "cool down") || strings.Contains(strings.ToLower(line), "cooldown") {
			// End any active interval block first
			if inIntervalBlock && currentBlock.Reps > 0 && len(blockSteps) > 0 {
				onStep := blockSteps[0]
				offStep := WorkoutStep{Duration: 60, PowerLow: 0.4}
				if len(blockSteps) > 1 {
					offStep = blockSteps[1]
				}
				steps = append(steps, WorkoutStep{
					Type:        "interval",
					Reps:        currentBlock.Reps,
					OnDuration:  onStep.Duration,
					OffDuration: offStep.Duration,
					OnPower:     onStep.PowerLow,
					OffPower:    offStep.PowerLow,
				})
				inIntervalBlock = false
				currentBlock = IntervalBlock{}
				blockSteps = []WorkoutStep{}
			}
			// Now add cooldown to main steps
			step := parseCooldownLine(line)
			steps = append(steps, step)
			continue
		}
		
		// Parse interval rep line: "• 0.25mi at 10:25/mi, 90s walking rest"
		if strings.HasPrefix(line, "•") && inIntervalBlock {
			step := parseIntervalRepLine(line)
			blockSteps = append(blockSteps, step)
			
			// Extract rest from same line
			restMatch := reInlineRest.FindStringSubmatch(line)
			if len(restMatch) > 1 {
				restDur, _ := strconv.Atoi(restMatch[1])
				blockSteps = append(blockSteps, WorkoutStep{Type: "rest", Duration: restDur, PowerLow: 0.4})
			}
			continue
		}
		
		// Parse steady state / progressive run lines
		// Must check BEFORE standalone rest to handle inline rest
		if strings.Contains(line, "mi") && strings.Contains(line, "at") && !strings.Contains(line, " • ") {
			step := parseSteadyStateLine(line)
			if inIntervalBlock {
				blockSteps = append(blockSteps, step)
			} else {
				steps = append(steps, step)
			}
			
			// Check for inline rest: ", 150s walking rest"
			restMatch := reInlineRest.FindStringSubmatch(line)
			if len(restMatch) > 1 {
				restDur, _ := strconv.Atoi(restMatch[1])
				steps = append(steps, WorkoutStep{Type: "rest", Duration: restDur})
			}
			continue
		}
		
		// Skip general rest mentions not already handled
		if strings.Contains(strings.ToLower(line), "rest") {
			continue
		}
	}
	
	// Handle any remaining interval block
	if inIntervalBlock && currentBlock.Reps > 0 && len(blockSteps) > 0 {
		onStep := blockSteps[0]
		offStep := WorkoutStep{Duration: 60, PowerLow: 0.4}
		if len(blockSteps) > 1 {
			offStep = blockSteps[1]
		}
		steps = append(steps, WorkoutStep{
			Type:        "interval",
			Reps:        currentBlock.Reps,
			OnDuration:  onStep.Duration,
			OffDuration: offStep.Duration,
			OnPower:     onStep.PowerLow,
			OffPower:    offStep.PowerLow,
		})
	}
	
	return steps
}

// parseWarmupLine parses warmup lines
func parseWarmupLine(line string) WorkoutStep {
	step := WorkoutStep{Type: "warmup"}
	
	distMatch := reDistance.FindStringSubmatch(line)
	if len(distMatch) > 1 {
		step.Distance = parseDistance(distMatch[1] + "mi")
	}
	
	paceMatch := rePaceLimit.FindStringSubmatch(line)
	if len(paceMatch) > 1 {
		step.PaceSecPerMile = parsePace(paceMatch[1])
	} else {
		step.PaceSecPerMile = ConversationalPace
	}
	
	// Calculate duration: distance × pace
	if step.Distance > 0 && step.PaceSecPerMile > 0 {
		step.Duration = int(step.Distance * float64(step.PaceSecPerMile))
	}
	
	power := paceToPower(step.PaceSecPerMile)
	step.PowerLow = power - 0.1
	if step.PowerLow < 0.35 {
		step.PowerLow = 0.35
	}
	step.PowerHigh = power
	
	return step
}

// parseCooldownLine parses cooldown lines
func parseCooldownLine(line string) WorkoutStep {
	step := WorkoutStep{Type: "cooldown"}
	
	distMatch := reDistance.FindStringSubmatch(line)
	if len(distMatch) > 1 {
		step.Distance = parseDistance(distMatch[1] + "mi")
	}
	
	paceMatch := rePaceLimit.FindStringSubmatch(line)
	if len(paceMatch) > 1 {
		step.PaceSecPerMile = parsePace(paceMatch[1])
	} else {
		step.PaceSecPerMile = ConversationalPace
	}
	
	if step.Distance > 0 && step.PaceSecPerMile > 0 {
		step.Duration = int(step.Distance * float64(step.PaceSecPerMile))
	}
	
	power := paceToPower(step.PaceSecPerMile)
	step.PowerLow = power
	step.PowerHigh = power - 0.1
	if step.PowerHigh < 0.35 {
		step.PowerHigh = 0.35
	}
	
	return step
}

// parseIntervalRepLine parses interval repetition lines
func parseIntervalRepLine(line string) WorkoutStep {
	step := WorkoutStep{Type: "interval_rep"}
	line = strings.TrimPrefix(line, "•")
	
	distMatch := reDistance.FindStringSubmatch(line)
	if len(distMatch) > 1 {
		step.Distance = parseDistance(distMatch[1] + "mi")
	}
	
	paceMatch := rePaceAt.FindStringSubmatch(line)
	if len(paceMatch) > 1 {
		step.PaceSecPerMile = parsePace(paceMatch[1])
	}
	
	if step.Distance > 0 && step.PaceSecPerMile > 0 {
		step.Duration = int(step.Distance * float64(step.PaceSecPerMile))
	}
	
	step.PowerLow = paceToPower(step.PaceSecPerMile)
	step.PowerHigh = step.PowerLow
	
	return step
}

// parseSteadyStateLine parses steady state / progressive run lines
func parseSteadyStateLine(line string) WorkoutStep {
	step := WorkoutStep{Type: "steady"}
	
	distMatch := reDistance.FindStringSubmatch(line)
	if len(distMatch) > 1 {
		step.Distance = parseDistance(distMatch[1] + "mi")
	}
	
	paceMatch := rePaceAt.FindStringSubmatch(line)
	if len(paceMatch) > 1 {
		step.PaceSecPerMile = parsePace(paceMatch[1])
	} else if containsAnyWord(line, "conversational", "easy", "whatever pace", "comfortable", "relaxed", "recovery") {
		step.PaceSecPerMile = ConversationalPace
	}
	
	if step.Distance > 0 && step.PaceSecPerMile > 0 {
		step.Duration = int(step.Distance * float64(step.PaceSecPerMile))
	}
	
	step.PowerLow = paceToPower(step.PaceSecPerMile)
	step.PowerHigh = step.PowerLow
	
	return step
}

// generateZWO generates ZWO XML from workout steps
func generateZWO(name string, steps []WorkoutStep) string {
	var sb strings.Builder
	
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<workout_file>` + "\n")
	sb.WriteString(fmt.Sprintf("  <name>%s</name>\n", escapeXML(name)))
	sb.WriteString("  <sportType>run</sportType>\n")
	sb.WriteString("  <workout>\n")
	
	for _, step := range steps {
		switch step.Type {
		case "warmup":
			sb.WriteString(fmt.Sprintf("    <Warmup Duration=\"%d\" PowerLow=\"% .2f\" PowerHigh=\"% .2f\" pace=\"0\"/>\n",
				step.Duration, step.PowerLow, step.PowerHigh))
		case "cooldown":
			sb.WriteString(fmt.Sprintf("    <Cooldown Duration=\"%d\" PowerLow=\"% .2f\" PowerHigh=\"% .2f\" pace=\"0\"/>\n",
				step.Duration, step.PowerLow, step.PowerHigh))
		case "steady":
			sb.WriteString(fmt.Sprintf("    <SteadyState Duration=\"%d\" Power=\"% .2f\" pace=\"0\"/>\n",
				step.Duration, step.PowerLow))
		case "rest":
			sb.WriteString(fmt.Sprintf("    <FreeRide Duration=\"%d\"/>\n", step.Duration))
		case "interval":
			sb.WriteString(fmt.Sprintf("    <IntervalsT Repeat=\"%d\" OnDuration=\"%d\" OffDuration=\"%d\" OnPower=\"% .2f\" OffPower=\"% .2f\" pace=\"0\"/>\n",
				step.Reps, step.OnDuration, step.OffDuration, step.OnPower, step.OffPower))
		}
	}
	
	sb.WriteString("  </workout>\n")
	sb.WriteString("</workout_file>\n")
	
	return sb.String()
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// ParseICal parses iCal content into events
// FIX #1: Line unfolding MUST happen BEFORE any regex field parsing
// FIX #2: DTSTART regex must handle timezone-qualified dates like 'DTSTART;TZID=America/New_York:20260322T080000'
func ParseICal(content string) []iCalEvent {
	var events []iCalEvent
	
	// Step 1: Unfold lines according to iCal RFC 5545
	// Folded lines: CRLF followed by space/tab on continuation line
	// Unfold by removing CRLF and the leading whitespace of continuation lines
	unfoldedContent := unfoldICalLines(content)
	
	// Step 2: Split into event blocks
	eventBlocks := strings.Split(unfoldedContent, "BEGIN:VEVENT")
	
	for _, block := range eventBlocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		if !strings.Contains(block, "END:VEVENT") {
			continue
		}
		
		event := iCalEvent{}
		
		// Extract UID - match UID: followed by value until newline or end
		uidMatch := reUID.FindStringSubmatch(block)
		if len(uidMatch) > 1 {
			event.UID = strings.TrimSpace(uidMatch[1])
		}
		
		// Extract SUMMARY
		summaryMatch := reSummary.FindStringSubmatch(block)
		if len(summaryMatch) > 1 {
			event.Summary = strings.TrimSpace(summaryMatch[1])
		}
		
		// Extract DESCRIPTION - handle multiline descriptions
		descMatch := reDescription.FindStringSubmatch(block)
		if len(descMatch) > 1 {
			desc := strings.TrimSpace(descMatch[1])
			// Unescape iCal characters
			desc = strings.ReplaceAll(desc, "\\n", "\n")
			desc = strings.ReplaceAll(desc, "\\,", ",")
			desc = strings.ReplaceAll(desc, "\\;", ";")
			desc = strings.ReplaceAll(desc, "\\\\", "\\")
			event.Description = desc
		}
		
		// FIX #2: Handle DTSTART with optional TZID parameter
		// Pattern: DTSTART or DTSTART;TZID=... or DTSTART;VALUE=DATE:...
		// Capture everything after the colon until newline
		startMatch := reDTStart.FindStringSubmatch(block)
		if len(startMatch) > 1 {
			dateValue := strings.TrimSpace(startMatch[1])
			dateValue = extractDateTimeValue(dateValue)
			// Parse in local timezone so date comparisons work correctly
			loc := time.Now().Location()
			if len(dateValue) == 8 {
				event.StartDate, _ = time.ParseInLocation("20060102", dateValue, loc)
			} else if len(dateValue) == 15 {
				event.StartDate, _ = time.ParseInLocation("20060102T150405", dateValue, loc)
			}
		}
		
		// Same fix for DTEND
		endMatch := reDTEnd.FindStringSubmatch(block)
		if len(endMatch) > 1 {
			endValue := strings.TrimSpace(endMatch[1])
			endValue = extractDateTimeValue(endValue)
			loc := time.Now().Location()
			if len(endValue) == 8 {
				event.EndDate, _ = time.ParseInLocation("20060102", endValue, loc)
			} else if len(endValue) == 15 {
				event.EndDate, _ = time.ParseInLocation("20060102T150405", endValue, loc)
			}
		}
		
		if event.UID != "" {
			events = append(events, event)
		}
	}
	
	return events
}

// unfoldICalLines unfolds folded iCal lines per RFC 5545
// Folded lines have CRLF followed by space/tab on the continuation line
func unfoldICalLines(content string) string {
	// Replace CRLF+space or CRLF+tab with nothing (unfold)
	// Also handle LF+space for Unix line endings
	result := content
	result = strings.ReplaceAll(result, "\r\n ", "")
	result = strings.ReplaceAll(result, "\r\n\t", "")
	result = strings.ReplaceAll(result, "\n ", "")
	result = strings.ReplaceAll(result, "\n\t", "")
	return result
}

// extractDateTimeValue helps extract just the date/time part from strings that may contain extra characters
func extractDateTimeValue(input string) string {
    // Remove any non-digit or non-T characters that might be appended
    var result strings.Builder
    for _, r := range input {
        if (r >= '0' && r <= '9') || r == 'T' {
            result.WriteRune(r)
        }
    }
    return result.String()
}

// fetchICal fetches the iCal feed
func fetchICal(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	
	return string(body), nil
}

// uploadResponse represents the JSON response from Intervals.icu
type uploadResponse struct {
	ID json.Number `json:"id"`
}

// containsAnyWord checks if a line contains any of the given keywords (case-insensitive).
func containsAnyWord(line string, words ...string) bool {
	lower := strings.ToLower(line)
	for _, w := range words {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// paceSecPerKm converts pace in sec/mile to sec/km
func paceSecPerKm(secPerMile int) int {
	return int(float64(secPerMile) / 1.60934)
}

// powerToPace is the reverse of paceToPower — converts a power zone fraction
// back to an approximate pace in sec/mile.
func powerToPace(power float64) int {
	switch {
	case power >= 1.0:
		return 570 // ~9:30/mi (interval/fast)
	case power >= 0.95:
		return 630 // ~10:30/mi (threshold)
	case power >= 0.8:
		return 695 // ~11:35/mi (tempo)
	case power >= 0.7:
		return 750 // ~12:30/mi (steady)
	default:
		return 785 // ~13:05/mi (easy)
	}
}

// paceToRunningPower estimates running power in watts from pace.
// Uses the formula: Power ≈ weight_kg × speed_m/s × CoR (cost of running).
// CoR for flat running is approximately 0.98-1.04 kJ/kg/km.
// With FTP=215w, we calibrate against the athlete's threshold pace.
// computePowerFromConfig calculates FTP from weight and 5K target using Stryd formula.
func computePowerFromConfig(cfg Config) (weightKg, ftp float64) {
	weightKg = cfg.WeightKg
	fiveKSec := cfg.TargetFiveKMin * 60
	speed := 5000.0 / fiveKSec             // m/s
	fiveKPowerPerKg := speed * 1.04         // Stryd ECOR
	cpPerKg := fiveKPowerPerKg / 1.038      // 5K = 103.8% of CP
	ftp = cpPerKg * weightKg
	return
}

// paceToRunningPower estimates running power from pace using Stryd's formula:
// Power (W) = weight_kg × speed_m/s × ECOR (1.04) / CP_ratio (1.038)
// This gives the CP-equivalent power at any pace.
func paceToRunningPower(secPerMile int) int {
	if secPerMile <= 0 || athleteWeightKg == 0 {
		return int(runFTP * 0.6)
	}
	speed := 1609.34 / float64(secPerMile) // m/s
	power := athleteWeightKg * speed * 1.04 / 1.038
	return int(power)
}

// buildWorkoutDoc converts workout steps into an Intervals.icu workout_doc
// with absolute pace targets in sec/km (shows real pace on Garmin).
func buildWorkoutDoc(steps []WorkoutStep) map[string]interface{} {
	var docSteps []map[string]interface{}
	for _, step := range steps {
		s := map[string]interface{}{}
		switch step.Type {
		case "warmup":
			s["warmup"] = true
			s["duration"] = step.Duration
			s["ramp"] = true
			slowPace := step.PaceSecPerMile + 60
			s["pace"] = map[string]interface{}{
				"units": "/km",
				"start": paceSecPerKm(slowPace),
				"end":   paceSecPerKm(step.PaceSecPerMile),
			}
			s["power"] = map[string]interface{}{
				"units": "w",
				"start": paceToRunningPower(slowPace),
				"end":   paceToRunningPower(step.PaceSecPerMile),
			}
		case "cooldown":
			s["cooldown"] = true
			s["duration"] = step.Duration
			s["ramp"] = true
			slowPace := step.PaceSecPerMile + 60
			s["pace"] = map[string]interface{}{
				"units": "/km",
				"start": paceSecPerKm(step.PaceSecPerMile),
				"end":   paceSecPerKm(slowPace),
			}
			s["power"] = map[string]interface{}{
				"units": "w",
				"start": paceToRunningPower(step.PaceSecPerMile),
				"end":   paceToRunningPower(slowPace),
			}
		case "steady":
			s["duration"] = step.Duration
			s["pace"] = map[string]interface{}{
				"units": "/km",
				"value": paceSecPerKm(step.PaceSecPerMile),
			}
			s["power"] = map[string]interface{}{
				"units": "w",
				"value": paceToRunningPower(step.PaceSecPerMile),
			}
		case "rest":
			s["duration"] = step.Duration
			s["freeride"] = true
		case "interval":
			s["reps"] = step.Reps
			onPace := powerToPace(step.OnPower)
			s["steps"] = []map[string]interface{}{
				{
					"duration": step.OnDuration,
					"pace": map[string]interface{}{
						"units": "/km",
						"value": paceSecPerKm(onPace),
					},
					"power": map[string]interface{}{
						"units": "w",
						"value": paceToRunningPower(onPace),
					},
				},
				{
					"duration": step.OffDuration,
					"freeride": true,
				},
			}
		}
		if len(s) > 0 {
			docSteps = append(docSteps, s)
		}
	}
	return map[string]interface{}{"steps": docSteps}
}

// uploadWorkout uploads a workout to Intervals.icu using workout_doc with absolute pace targets.
func uploadWorkout(event iCalEvent, steps []WorkoutStep) (string, error) {
	workoutDoc := buildWorkoutDoc(steps)

	payload := map[string]interface{}{
		"category":         "WORKOUT",
		"show_as_note":     false,
		"start_date_local": event.StartDate.Format("2006-01-02") + "T08:00:00",
		"name":             event.Summary,
		"type":             "Run",
		"workout_doc":      workoutDoc,
	}
	payloadBytes, _ := json.Marshal(payload)
	
	url := fmt.Sprintf("%s/athlete/%s/events", IntervalsICUBase, AthleteID)
	
	req, err := http.NewRequest("POST", url, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return "", err
	}
	
	req.SetBasicAuth(APIKey, APIToken)
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	// Parse JSON response properly
	var result uploadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v, body: %s", err, string(body))
	}
	
	idStr := result.ID.String()
	if idStr == "" || idStr == "0" {
		return "", fmt.Errorf("no event ID in response: %s", string(body))
	}
	
	return idStr, nil
}

// existingEvent represents an event already on the Intervals.icu calendar.
type existingEvent struct {
	ID             json.Number `json:"id"`
	Name           string      `json:"name"`
	Category       string      `json:"category"`
	StartDateLocal string      `json:"start_date_local"`
}

// fetchExistingEvents returns all workout events in the date range, keyed by date "YYYY-MM-DD".
// Only one Runna workout per day — if multiple exist on the same date, keeps the first and
// deletes extras to prevent duplicates.
func fetchExistingEvents(startDate, endDate time.Time) (map[string]string, error) {
	url := fmt.Sprintf("%s/athlete/%s/events?oldest=%s&newest=%s",
		IntervalsICUBase, AthleteID,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(APIKey, APIToken)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var events []existingEvent
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events: %v", err)
	}

	// Key by date only — one workout per day max.
	// If duplicates exist on the same date, delete extras.
	result := make(map[string]string)
	client := &http.Client{}
	for _, e := range events {
		if e.Category != "WORKOUT" {
			continue
		}
		date := e.StartDateLocal
		if len(date) >= 10 {
			date = date[:10]
		}
		if _, exists := result[date]; exists {
			// Duplicate on this date — delete it
			delURL := fmt.Sprintf("%s/athlete/%s/events/%s", IntervalsICUBase, AthleteID, e.ID.String())
			delReq, _ := http.NewRequest("DELETE", delURL, nil)
			delReq.SetBasicAuth(APIKey, APIToken)
			delResp, err := client.Do(delReq)
			if err == nil {
				delResp.Body.Close()
				fmt.Printf("  Deleted duplicate on %s: %s (ID: %s)\n", date, e.Name, e.ID.String())
			}
			continue
		}
		result[date] = e.ID.String()
	}
	return result, nil
}

// updateWorkout updates an existing workout via PUT.
func updateWorkout(eventID string, steps []WorkoutStep) error {
	workoutDoc := buildWorkoutDoc(steps)
	payload := map[string]interface{}{
		"workout_doc": workoutDoc,
	}
	payloadBytes, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/athlete/%s/events/%s", IntervalsICUBase, AthleteID, eventID)
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(APIKey, APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

type IntervalBlock struct {
	Reps       int
	OnDistance float64
	OnPace     int
	OffDuration int
}

// recentActivity represents a completed run from Intervals.icu
type recentActivity struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Type           string      `json:"type"`
	StartDateLocal string      `json:"start_date_local"`
	Distance       float64     `json:"distance"`       // meters
	MovingTime     int         `json:"moving_time"`     // seconds
	AverageSpeed   float64     `json:"average_speed"`   // m/s
	AverageWatts   json.Number `json:"icu_average_watts"`
	MaxWatts       json.Number `json:"icu_max_watts"`
	EFTP           json.Number `json:"icu_eftp"`
}

// fetchRecentActivities gets completed runs from the last N days
func fetchRecentActivities(days int) ([]recentActivity, error) {
	now := time.Now()
	oldest := now.AddDate(0, 0, -days).Format("2006-01-02")
	newest := now.Format("2006-01-02")
	url := fmt.Sprintf("%s/athlete/%s/activities?oldest=%s&newest=%s", IntervalsICUBase, AthleteID, oldest, newest)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(APIKey, APIToken)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var activities []recentActivity
	if err := json.Unmarshal(body, &activities); err != nil {
		return nil, err
	}

	// Filter to runs only
	var runs []recentActivity
	for _, a := range activities {
		if a.Type == "Run" || a.Type == "VirtualRun" || a.Type == "TrailRun" {
			if a.Distance > 0 && a.MovingTime > 0 {
				runs = append(runs, a)
			}
		}
	}
	return runs, nil
}

// updateSportSettings updates FTP and/or threshold pace on Intervals.icu
func updateSportSettings(sportSettingsID int, ftp *int, thresholdPace *float64) error {
	payload := map[string]interface{}{}
	if ftp != nil {
		payload["ftp"] = *ftp
	}
	if thresholdPace != nil {
		payload["threshold_pace"] = *thresholdPace
	}
	payloadBytes, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/athlete/%s/sport-settings/%d", IntervalsICUBase, AthleteID, sportSettingsID)
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(APIKey, APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// getRunSportSettingsID fetches the sport settings ID for Run
func getRunSportSettingsID() (int, error) {
	url := fmt.Sprintf("%s/athlete/%s/sport-settings", IntervalsICUBase, AthleteID)
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(APIKey, APIToken)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var settings []struct {
		ID    int      `json:"id"`
		Types []string `json:"types"`
	}
	if err := json.Unmarshal(body, &settings); err != nil {
		return 0, err
	}
	for _, s := range settings {
		for _, t := range s.Types {
			if t == "Run" {
				return s.ID, nil
			}
		}
	}
	return 0, fmt.Errorf("Run sport settings not found")
}

// cmdUpdateFromPace analyzes recent runs, estimates FTP from best pace,
// updates threshold pace AND power on Intervals.icu, then re-syncs workouts.
// Mode 1: pace → pace (updates threshold pace from best effort)
// Mode 2: pace → power (updates FTP from pace using Stryd formula)
func cmdUpdateFromPace(lookbackDays int) {
	fmt.Printf("Analyzing last %d days of runs...\n", lookbackDays)
	runs, err := fetchRecentActivities(lookbackDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching activities: %v\n", err)
		return
	}
	if len(runs) == 0 {
		fmt.Println("No runs found in the last", lookbackDays, "days")
		return
	}

	// Find best 5K-equivalent effort (fastest average pace on runs >= 3km)
	var bestSpeed float64
	var bestRun recentActivity
	for _, r := range runs {
		if r.Distance < 3000 { // need at least 3km for meaningful pace
			continue
		}
		speed := r.Distance / float64(r.MovingTime)
		if speed > bestSpeed {
			bestSpeed = speed
			bestRun = r
		}
	}

	if bestSpeed == 0 {
		fmt.Println("No qualifying runs (>3km) found")
		return
	}

	// Convert best speed to pace
	bestPaceSecPerMile := 1609.34 / bestSpeed
	bestPaceMins := int(bestPaceSecPerMile) / 60
	bestPaceSecs := int(bestPaceSecPerMile) % 60

	// Estimate 5K time from this pace (adjust for distance — longer runs are slower)
	// Use Riegel formula: T2 = T1 * (D2/D1)^1.06
	actualTimeSec := float64(bestRun.MovingTime)
	actualDistM := bestRun.Distance
	estimated5KTime := actualTimeSec * math.Pow(5000.0/actualDistM, 1.06)
	estimated5KMin := estimated5KTime / 60.0

	// Threshold pace from estimated 5K
	thresholdSpeed := 5000.0 / estimated5KTime // m/s
	thresholdPaceSecPerMile := 1609.34 / thresholdSpeed
	tpMins := int(thresholdPaceSecPerMile) / 60
	tpSecs := int(thresholdPaceSecPerMile) % 60

	// FTP from Stryd formula
	newFTP := int(athleteWeightKg * thresholdSpeed * 1.04 / 1.038)

	fmt.Printf("\nBest effort: %s (%s)\n", bestRun.Name, bestRun.StartDateLocal[:10])
	fmt.Printf("  Distance: %.0fm, Time: %dm%ds, Pace: %d:%02d/mi\n",
		bestRun.Distance, bestRun.MovingTime/60, bestRun.MovingTime%60, bestPaceMins, bestPaceSecs)
	fmt.Printf("  Estimated 5K time: %.1f min\n", estimated5KMin)
	fmt.Printf("  New threshold pace: %d:%02d/mi (%.3f m/s)\n", tpMins, tpSecs, thresholdSpeed)
	fmt.Printf("  New FTP: %dw (was %.0fw)\n", newFTP, runFTP)

	// Update sport settings
	ssID, err := getRunSportSettingsID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sport settings: %v\n", err)
		return
	}

	if err := updateSportSettings(ssID, &newFTP, &thresholdSpeed); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating sport settings: %v\n", err)
		return
	}
	fmt.Printf("  ✅ Updated Intervals.icu: FTP=%dw, threshold=%.3f m/s\n", newFTP, thresholdSpeed)

	// Update runtime values for workout re-sync
	runFTP = float64(newFTP)
	cfg.TargetFiveKMin = estimated5KMin
}

// cmdUpdateFromPower reads actual power data from recent Stryd-equipped runs
// and updates FTP directly from measured power (no pace estimation needed).
// Mode 3: power → power
func cmdUpdateFromPower(lookbackDays int) {
	fmt.Printf("Analyzing last %d days of runs for power data...\n", lookbackDays)
	runs, err := fetchRecentActivities(lookbackDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching activities: %v\n", err)
		return
	}

	// Find runs with actual power data
	var bestPower float64
	var bestRun recentActivity
	for _, r := range runs {
		if r.Distance < 3000 {
			continue
		}
		watts, err := r.AverageWatts.Float64()
		if err != nil || watts == 0 {
			continue
		}
		// Longer efforts at high power are more meaningful
		// Use power × sqrt(duration) as a quality metric
		quality := watts * math.Sqrt(float64(r.MovingTime))
		bestQuality := bestPower * math.Sqrt(float64(bestRun.MovingTime))
		if quality > bestQuality {
			bestPower = watts
			bestRun = r
		}
	}

	if bestPower == 0 {
		fmt.Println("No runs with power data found. Do you have a Stryd pod?")
		fmt.Println("Falling back to pace-based estimation...")
		cmdUpdateFromPace(lookbackDays)
		return
	}

	// 5K race power is ~103.8% of CP, so adjust based on run duration
	durationMin := float64(bestRun.MovingTime) / 60.0
	// Riegel-style power decay: shorter = higher % of CP
	var cpRatio float64
	switch {
	case durationMin < 15:
		cpRatio = 1.06 // ~5K effort
	case durationMin < 30:
		cpRatio = 1.038 // ~5K equivalent
	case durationMin < 60:
		cpRatio = 1.00 // ~threshold
	default:
		cpRatio = 0.95 // sub-threshold
	}
	newFTP := int(bestPower / cpRatio)

	// Derive threshold pace from new FTP
	cpPerKg := float64(newFTP) / athleteWeightKg
	thresholdSpeed := cpPerKg / 1.04 * 1.038 // reverse Stryd formula
	thresholdPaceSecPerMile := 1609.34 / thresholdSpeed
	tpMins := int(thresholdPaceSecPerMile) / 60
	tpSecs := int(thresholdPaceSecPerMile) % 60

	fmt.Printf("\nBest power effort: %s (%s)\n", bestRun.Name, bestRun.StartDateLocal[:10])
	fmt.Printf("  Avg power: %.0fw, Duration: %dm, Distance: %.0fm\n",
		bestPower, bestRun.MovingTime/60, bestRun.Distance)
	fmt.Printf("  CP ratio: %.3f (based on %dm effort)\n", cpRatio, bestRun.MovingTime/60)
	fmt.Printf("  New FTP: %dw (was %.0fw)\n", newFTP, runFTP)
	fmt.Printf("  Derived threshold pace: %d:%02d/mi (%.3f m/s)\n", tpMins, tpSecs, thresholdSpeed)

	ssID, err := getRunSportSettingsID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sport settings: %v\n", err)
		return
	}

	if err := updateSportSettings(ssID, &newFTP, &thresholdSpeed); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating sport settings: %v\n", err)
		return
	}
	fmt.Printf("  ✅ Updated Intervals.icu: FTP=%dw, threshold=%.3f m/s\n", newFTP, thresholdSpeed)

	runFTP = float64(newFTP)
}

func main() {
	cfg = loadConfig()

	// Validate required config
	if cfg.AthleteID == "" {
		fmt.Fprintln(os.Stderr, "Error: INTERVALS_ATHLETE_ID environment variable is required")
		os.Exit(1)
	}
	if cfg.APIToken == "" {
		fmt.Fprintln(os.Stderr, "Error: INTERVALS_API_TOKEN environment variable is required")
		os.Exit(1)
	}

	// Set convenience aliases
	IntervalsICUBase = cfg.IntervalsICUBase
	AthleteID = cfg.AthleteID
	APIKey = cfg.APIKey
	APIToken = cfg.APIToken
	RunnaICalURL = cfg.RunnaICalURL

	// Compute power from config
	athleteWeightKg, runFTP = computePowerFromConfig(cfg)

	// Subcommands
	cmd := "sync"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "update-pace":
		// Mode 1+2: pace→pace and pace→power
		// Analyzes recent runs, updates threshold pace AND FTP from best pace
		fmt.Printf("Config: %s, weight=%.1fkg, current FTP=%.0fw\n", cfg.AthleteID, athleteWeightKg, runFTP)
		lookback := 30
		if len(os.Args) > 2 {
			if d, err := strconv.Atoi(os.Args[2]); err == nil {
				lookback = d
			}
		}
		cmdUpdateFromPace(lookback)
		// After updating, re-sync workouts with new targets
		fmt.Println("\nRe-syncing workouts with updated targets...")
		cmd = "sync"
	case "update-power":
		// Mode 3: power→power
		// Reads actual Stryd power data, updates FTP from measured power
		fmt.Printf("Config: %s, weight=%.1fkg, current FTP=%.0fw\n", cfg.AthleteID, athleteWeightKg, runFTP)
		lookback := 30
		if len(os.Args) > 2 {
			if d, err := strconv.Atoi(os.Args[2]); err == nil {
				lookback = d
			}
		}
		cmdUpdateFromPower(lookback)
		fmt.Println("\nRe-syncing workouts with updated targets...")
		cmd = "sync"
	case "sync":
		fmt.Printf("Config: %s, weight=%.1fkg, FTP=%.0fw (%.1fmin 5K)\n",
			cfg.AthleteID, athleteWeightKg, runFTP, cfg.TargetFiveKMin)
	default:
		fmt.Fprintf(os.Stderr, "Usage: runna-sync [sync|update-pace|update-power] [days]\n")
		fmt.Fprintf(os.Stderr, "  sync          Sync Runna workouts to Intervals.icu (default)\n")
		fmt.Fprintf(os.Stderr, "  update-pace   Update FTP+pace from recent run paces, then sync\n")
		fmt.Fprintf(os.Stderr, "  update-power  Update FTP from Stryd power data, then sync\n")
		os.Exit(1)
	}

	if cmd != "sync" {
		return
	}

	if cfg.RunnaICalURL == "" {
		fmt.Fprintln(os.Stderr, "Error: RUNNA_ICAL_URL environment variable is required for sync")
		os.Exit(1)
	}

	syncDays := cfg.SyncDays
	if len(os.Args) > 1 {
		if days, err := strconv.Atoi(os.Args[len(os.Args)-1]); err == nil && days > 0 {
			syncDays = days
		}
	}
	
	fmt.Printf("Fetching Runna training plan from iCal...\n")
	
	icalContent, err := fetchICal(RunnaICalURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching iCal: %v\n", err)
		os.Exit(1)
	}
	
	events := ParseICal(icalContent)
	fmt.Printf("Found %d events in iCal feed\n", len(events))
	
	now := time.Now()
	// Use start of today so today's workouts are included
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endDate := today.AddDate(0, 0, syncDays)

	var filteredEvents []iCalEvent
	for _, event := range events {
		if !event.StartDate.Before(today) && event.StartDate.Before(endDate) {
			filteredEvents = append(filteredEvents, event)
		}
	}
	
	fmt.Printf("Filtered to %d events in date range (%s to %s)\n",
		len(filteredEvents), now.Format("2006-01-02"), endDate.Format("2006-01-02"))
	
	if len(filteredEvents) == 0 {
		fmt.Println("No upcoming workouts to sync")
		return
	}

	// Fetch existing events for upsert matching
	fmt.Println("Checking existing workouts...")
	existing, err := fetchExistingEvents(today, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch existing events: %v\n", err)
		existing = make(map[string]string)
	}
	fmt.Printf("  Found %d existing events in range\n", len(existing))

	created, updated := 0, 0
	for _, event := range filteredEvents {
		fmt.Printf("\nProcessing: %s (%s)\n", event.Summary, event.StartDate.Format("2006-01-02"))

		if event.Description == "" {
			fmt.Println("  Skipping: no description")
			continue
		}

		steps := parseWorkoutDescription(event.Description)
		fmt.Printf("  Parsed %d steps\n", len(steps))

		// Check if a workout already exists on this date (one per day max)
		key := event.StartDate.Format("2006-01-02")
		if existingID, ok := existing[key]; ok {
			// Update existing workout in place
			if err := updateWorkout(existingID, steps); err != nil {
				fmt.Fprintf(os.Stderr, "  Update failed: %v\n", err)
				continue
			}
			fmt.Printf("  Updated existing (event ID: %s)\n", existingID)
			updated++
		} else {
			// Create new workout
			eventID, err := uploadWorkout(event, steps)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Upload failed: %v\n", err)
				continue
			}
			fmt.Printf("  Created new (event ID: %s)\n", eventID)
			created++
		}
	}

	fmt.Printf("\nSync complete! Created: %d, Updated: %d\n", created, updated)
}
