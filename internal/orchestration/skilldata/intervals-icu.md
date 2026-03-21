---
name: intervals-icu
description: "Intervals.icu API integration — structured workout creation, calendar events, Garmin sync via ZWO format."
triggers:
  - "intervals"
  - "intervals.icu"
  - "garmin"
  - "workout sync"
  - "structured workout"
  - "workout steps"
  - "training plan"
  - "zwo"
  - "runna"
  - "runna-sync"
role: coder
phase: implement
mcp_tools: []
verify:
  - name: "ZWO pace attribute exists"
    command: "grep -r 'pace' --include='*.go' | grep -i 'xml\\|attr' || echo 'MISSING: pace attribute not found in ZWO structs'"
    expect_not: "MISSING"
  - name: "ZWO sportType is run"
    command: "grep -r 'sportType\\|SportType' --include='*.go' | grep -i 'run' || echo 'MISSING'"
    expect_not: "MISSING"
  - name: "Basic Auth with API_KEY username"
    command: "grep -r 'SetBasicAuth' --include='*.go' || echo 'MISSING: no Basic Auth found'"
    expect_not: "MISSING"
  - name: "ZWO XML root element"
    command: "grep -r 'workout_file' --include='*.go' || echo 'MISSING: should use <workout_file> as root'"
    expect_not: "MISSING"
  - name: "duration calculation"
    command: "grep -rn 'Distance.*float64.*Pace\\|distance.*pace\\|Duration.*int' --include='*.go' | head -5 || echo 'CHECK: verify Duration = distance × pace'"
    manual: "Verify that workout step durations are calculated as distance_miles × pace_seconds_per_mile"
  - name: "cooldown ramp direction"
    command: "grep -B5 -A5 'Cooldown' --include='*.go' -r | grep 'PowerLow\\|PowerHigh'"
    manual: "For cooldowns, PowerLow should be HIGHER than PowerHigh (ramps DOWN). Verify the values."
  - name: "ZWO attribute casing (PascalCase)"
    command: "grep -rn 'xml:\"[a-z].*,attr' --include='*.go' | grep -i 'duration\\|power\\|pace\\|repeat' || echo 'OK'"
    expect: "OK"
    manual: "ZWO attributes must be PascalCase: Duration, PowerLow, PowerHigh, Power, Repeat, OnDuration, OffDuration, OnPower, OffPower. Lowercase will silently fail on Intervals.icu."
  - name: "iCal line folding handled"
    command: "grep -rn 'fold\\|unfold\\|CRLF\\|\\\\r\\\\n\\|continuation' --include='*.go' || echo 'MISSING'"
    manual: "RFC 5545 requires handling line folding (long lines split with CRLF + space). If parsing iCal manually, unfold lines before processing. Or use a library like golang-ical."
  - name: "iCal date format variants"
    command: "grep -rn 'DTSTART' --include='*.go' || echo 'CHECK'"
    manual: "iCal dates can be DTSTART:20260322, DTSTART;VALUE=DATE:20260322, DTSTART;TZID=America/New_York:20260322T080000. Verify all formats are handled, or use a library."
  - name: "duplicate event prevention"
    command: "grep -rn 'ListEvent\\|GetEvent\\|delete.*exist\\|duplicate\\|already' --include='*.go' || echo 'MISSING'"
    manual: "Running the sync tool twice should not create duplicate events. Check if existing events are queried before uploading, or if events are deleted/updated by UID."
  - name: "Event.ID handles numeric and string"
    command: "grep -rn 'json.Number\\|RawMessage\\|interface{}.*id\\|IDRaw' --include='*.go' || echo 'CHECK'"
    manual: "Intervals.icu returns event IDs as numbers (not strings). The JSON decoder must handle both types. Use json.RawMessage or json.Number for the ID field."
---

# Skill: Intervals.icu API Integration

Create structured workouts on Intervals.icu that sync to Garmin watches with proper warmup/interval/cooldown steps.

## Authentication

HTTP Basic Auth:
- Username: `API_KEY`
- Password: the API token from Intervals.icu Settings → Developer Settings
- Example: `curl -u "API_KEY:your_token" https://intervals.icu/api/v1/athlete/{id}/events`

**NOT Bearer token. NOT API key in header. Always Basic Auth.**

## Endpoints

| Action | Method | Endpoint |
|--------|--------|----------|
| List events | GET | `/api/v1/athlete/{id}/events?oldest=YYYY-MM-DD&newest=YYYY-MM-DD` |
| Create event | POST | `/api/v1/athlete/{id}/events` |
| Update event | PUT | `/api/v1/athlete/{id}/events/{eventId}` |
| Delete event | DELETE | `/api/v1/athlete/{id}/events/{eventId}` |
| Get athlete | GET | `/api/v1/athlete/{id}` |

## CRITICAL: How to Create Structured Workouts

**DO NOT put workout steps in the `description` field.** Text descriptions do NOT create proper structured steps.

**DO NOT manually construct `workout_doc.steps`.** That field is read-only server-generated metadata.

**USE ZWO (Zwift Workout) file upload.** This is the ONLY reliable way to create structured workouts with proper step types that sync to Garmin.

### ZWO Upload Format

```json
{
  "category": "WORKOUT",
  "show_as_note": false,
  "start_date_local": "2026-03-17T08:00:00",
  "name": "My Workout",
  "type": "Run",
  "file_contents_base64": "<base64-encoded ZWO XML>",
  "filename": "workout.zwo"
}
```

### ZWO XML Format

```xml
<?xml version="1.0" encoding="UTF-8"?>
<workout_file>
  <name>Workout Name</name>
  <sportType>run</sportType>
  <workout>
    <Warmup Duration="600" PowerLow="0.5" PowerHigh="0.7" pace="0"/>
    <IntervalsT Repeat="5" OnDuration="96" OffDuration="60" OnPower="0.95" OffPower="0.5" pace="0"/>
    <Cooldown Duration="600" PowerLow="0.5" PowerHigh="0.3" pace="0"/>
  </workout>
</workout_file>
```

### ZWO Step Types

| Element | Use | Key Attributes |
|---------|-----|----------------|
| `<Warmup>` | Warm-up period | `Duration` (seconds), `PowerLow`, `PowerHigh` |
| `<SteadyState>` | Steady effort | `Duration` (seconds), `Power` (0.0-2.0 as % FTP) |
| `<IntervalsT>` | Repeated intervals | `Repeat`, `OnDuration`, `OffDuration`, `OnPower`, `OffPower` |
| `<Cooldown>` | Cool-down period | `Duration` (seconds), `PowerLow`, `PowerHigh` |
| `<FreeRide>` | Open/easy effort | `Duration` (seconds) |

### Power Values in ZWO

Power is expressed as decimal fraction of FTP (Functional Threshold Power):
- 0.5 = 50% FTP (easy/recovery)
- 0.7 = 70% FTP (endurance)
- 0.85 = 85% FTP (tempo)
- 0.95 = 95% FTP (threshold)
- 1.0 = 100% FTP (threshold)
- 1.1 = 110% FTP (VO2max intervals)
- 1.2+ = 120%+ FTP (short hard efforts)

### Converting Runna Distances + Paces to ZWO Durations

**CRITICAL: Calculate Duration from distance and pace. Do NOT guess durations.**

Formula: `Duration (seconds) = distance_in_miles × pace_in_seconds_per_mile`

Examples:
- "1mi at 13:05/mi" → 1 × 785 = **785 seconds**
- "0.25mi at 10:30/mi" → 0.25 × 630 = **158 seconds**
- "2mi at 11:40/mi" → 2 × 700 = **1400 seconds**
- "3.25mi at 13:05/mi" → 3.25 × 785 = **2551 seconds**

Pace conversion: "MM:SS/mi" → total seconds. E.g., "13:05/mi" = 13×60 + 5 = 785 sec/mi

### Converting Runna Paces to ZWO Power

For runners without power meters, map pace zones to approximate power percentages:
- Conversational/easy pace (13:00+/mi) → 0.55-0.65
- Steady/moderate pace (11:30-12:30/mi) → 0.70-0.80
- Tempo pace (10:30-11:30/mi) → 0.85-0.90
- Threshold pace (9:30-10:30/mi) → 0.95-1.00
- Interval/fast pace (8:30-9:30/mi) → 1.00-1.10
- Sprint/hard pace (<8:30/mi) → 1.10-1.25
- Recovery/walking → 0.35-0.45

### ZWO Examples

**Easy Run (3.25mi at 13:05/mi = 2551s):**
```xml
<workout_file>
  <name>3.25mi Easy Run</name>
  <sportType>run</sportType>
  <workout>
    <SteadyState Duration="2551" Power="0.6" pace="0"/>
  </workout>
</workout_file>
```

**400m Repeats: "1mi warmup at 13:05/mi, 90s rest, 5x 0.25mi at 10:30/mi w/ 60s rest, 1mi cooldown"**
- 1mi at 13:05/mi = 785s warmup
- 90s walking rest
- 0.25mi at 10:30/mi = 158s per rep, 5 reps, 60s rest between
- 1mi at 13:05/mi = 785s cooldown
```xml
<workout_file>
  <name>400m Repeats</name>
  <sportType>run</sportType>
  <workout>
    <Warmup Duration="785" PowerLow="0.5" PowerHigh="0.6" pace="0"/>
    <FreeRide Duration="90"/>
    <IntervalsT Repeat="5" OnDuration="158" OffDuration="60" OnPower="0.95" OffPower="0.4" pace="0"/>
    <Cooldown Duration="785" PowerLow="0.6" PowerHigh="0.5" pace="0"/>
  </workout>
</workout_file>
```

**Tempo Run: "1mi warmup at 12:55/mi, 2mi at 11:40/mi, 150s rest, 0.25mi cooldown"**
- 1mi at 12:55/mi = 775s warmup
- 2mi at 11:40/mi = 1400s tempo
- 150s rest
- 0.25mi at 12:55/mi = 194s cooldown
```xml
<workout_file>
  <name>Tempo 2 Miles</name>
  <sportType>run</sportType>
  <workout>
    <Warmup Duration="775" PowerLow="0.5" PowerHigh="0.6" pace="0"/>
    <SteadyState Duration="1400" Power="0.88" pace="0"/>
    <FreeRide Duration="150"/>
    <Cooldown Duration="194" PowerLow="0.6" PowerHigh="0.5" pace="0"/>
  </workout>
</workout_file>
```

**Progressive Long Run: "2mi easy, 1mi at 11:55/mi, 1mi at 11:35/mi, 0.5mi easy"**
- 2mi at 13:00/mi = 1560s
- 1mi at 11:55/mi = 715s
- 1mi at 11:35/mi = 695s
- 0.5mi at 13:00/mi = 390s
```xml
<workout_file>
  <name>Progressive Long Run</name>
  <sportType>run</sportType>
  <workout>
    <SteadyState Duration="1560" Power="0.6" pace="0"/>
    <SteadyState Duration="715" Power="0.75" pace="0"/>
    <SteadyState Duration="695" Power="0.82" pace="0"/>
    <Cooldown Duration="390" PowerLow="0.6" PowerHigh="0.5" pace="0"/>
  </workout>
</workout_file>
```

## Verification After Upload

Always fetch the event back and verify:
```bash
curl -u "API_KEY:token" https://intervals.icu/api/v1/athlete/{id}/events/{eventId}
```

Check that `workout_doc.steps`:
- Is NOT empty
- Has steps with `warmup: true` or `cooldown: true` for those step types
- Has `reps` and nested `steps` for interval blocks
- Has `power` or `pace` targets on work steps

If `workout_doc.steps` is empty, the upload format was wrong.

## Garmin Sync

### Enable Garmin Workout Upload
```bash
curl -X PUT -u "API_KEY:token" \
  "https://intervals.icu/api/v1/athlete/{id}" \
  -H "Content-Type: application/json" \
  -d '{"icu_garmin_upload_workouts": true}'
```

- Once enabled, Intervals.icu auto-pushes the next week of planned workouts to Garmin Connect
- No API endpoint to force-push — sync happens automatically on Intervals.icu's schedule
- User syncs their Garmin watch with Garmin Connect to receive workouts on-device
- If Garmin connection is stale (>1 year), re-authorize at intervals.icu/settings

### Check Garmin Status
```bash
curl -u "API_KEY:token" "https://intervals.icu/api/v1/athlete/{id}" | \
  jq '{garmin_health: .icu_garmin_health, garmin_training: .icu_garmin_training,
       upload_workouts: .icu_garmin_upload_workouts, last_upload: .icu_garmin_last_upload}'
```

## Athlete Settings Reference

Key fields on the athlete object:
- `icu_garmin_upload_workouts` — boolean, push workouts to Garmin
- `pace_units` — "MINS_MILE" or "SECS_100M"
- `sportSettings[].ftp` — FTP per sport (Run FTP: 215w for this athlete)
- `sportSettings[].warmup_time` — default warmup in seconds
- `sportSettings[].cooldown_time` — default cooldown in seconds

## ZWO Display Limitations

ZWO format is power/duration-based, NOT distance/pace-based:
- Warmup/cooldown display as "13m5s 48.7% Pace (0mi)" — the 0mi is expected
- Steps show as `%Pace` relative to threshold, not absolute pace like "13:05/mi"
- `<Warmup>` and `<Cooldown>` always display as "ramp" even if PowerLow == PowerHigh
- Use `<SteadyState>` for flat display (but loses the warmup/cooldown label)
- Intervals.icu overwrites any custom `description` with its auto-generated one from ZWO

## Runna iCal Integration

Fetch Runna workouts from the iCal feed:
```bash
curl -s "https://cal.runna.com/{user_feed_id}.ics"
```

Parse VEVENT blocks for: SUMMARY, DESCRIPTION, DTSTART. Description contains structured workout text with distances and paces convertible to ZWO.

## Common Mistakes

1. **Using `description` for workout structure** — Only creates text notes, not structured steps
2. **Using Bearer auth** — Intervals.icu uses HTTP Basic Auth, not Bearer tokens
3. **Posting to `/workouts`** — Use `/events` for calendar placement
4. **Setting `show_as_note: true`** — Must be `false` for structured workouts
5. **Constructing `workout_doc` manually** — This field is read-only; use ZWO upload instead
6. **Using zone references (Z1, Z4)** — Only works if athlete has zones configured
7. **Trying to force Garmin sync via API** — No endpoint exists; enable `icu_garmin_upload_workouts` and it syncs automatically
8. **Using `<Warmup>` for steady pace** — Always displays as "ramp"; use `<SteadyState>` for flat effort
9. **Expecting distance display from ZWO** — ZWO is duration-based; distances show as 0mi in the UI
