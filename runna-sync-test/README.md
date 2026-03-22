# Runna to Intervals.icu Sync Tool

A Go tool that fetches your Runna training plan from the iCal feed, parses workout descriptions into structured steps, generates ZWO workout files, and uploads them to Intervals.icu for Garmin watch sync.

## Features

- Fetches workouts from Runna iCal feed
- Parses complex workout descriptions including:
  - Easy runs with conversational pace
  - Tempo runs with warmup/steady/cooldown
  - Progressive long runs with multiple pace zones
  - Interval workouts with single or multiple interval blocks
- Generates valid ZWO (Zwift Workout) XML files
- Uploads structured workouts to Intervals.icu
- Cleans up existing Runna events before re-upload
- Calculates exact durations from distance × pace

## Usage

```bash
# Sync next 14 days (default)
./runna-sync

# Sync next 30 days
./runna-sync 30

# Sync next 7 days
./runna-sync 7
```

## Configuration

Edit the constants in `main.go`:

```go
const (
    RunnaICalURL       = "https://cal.runna.com/YOUR_ICAL_ID.ics"
    AthleteID          = "i247251"
    APIKey             = "API_KEY"
    APIToken           = "your_token"
)
```

**Security Note:** In production, load credentials from environment variables instead of hardcoding.

## How It Works

1. **Fetch iCal**: Downloads the Runna training plan from the public iCal URL
2. **Parse Events**: Extracts workout events within the specified date range
3. **Parse Descriptions**: Converts text descriptions into structured steps:
   - Warmup steps with duration calculated from distance × pace
   - Steady state efforts with power targets
   - Interval blocks with reps, on/off durations
   - Rest periods as FreeRide steps
   - Cooldown steps
4. **Generate ZWO**: Creates Zwift-format XML with proper step types
5. **Upload**: POSTs to Intervals.icu API with base64-encoded ZWO file
6. **Cleanup**: Removes existing Runna events in the date range to avoid duplicates

## Workout Parsing Examples

### Easy Run
```
3.25mi easy run at a conversational pace (no faster than 13:05/mi)
→ SteadyState: 2551s (3.25 × 785 sec/mi) at 60% FTP
```

### Tempo Run
```
1mi warm up at 12:55/mi
2mi at 11:40/mi, 150s walking rest
0.25mi cool down
→ Warmup: 775s, SteadyState: 1400s, FreeRide: 150s, Cooldown: 196s
```

### Interval Workout (Multiple Blocks)
```
0.75mi warm up
3 reps of:
  • 0.25mi at 10:25/mi, 90s rest
60s walking rest
3 reps of:
  • 0.12mi at 10:00/mi, 60s rest
0.25mi cool down
→ Warmup, IntervalsT(3×156s/90s), FreeRide(60s), IntervalsT(3×72s/60s), Cooldown
```

## Duration Calculations

All durations are calculated exactly:
```
Duration (seconds) = distance_miles × pace_seconds_per_mile

Examples:
- 1mi at 13:05/mi  → 1 × 785  = 785s
- 0.25mi at 10:30/mi → 0.25 × 630 = 157s
- 2mi at 11:40/mi  → 2 × 700  = 1400s
- 3.25mi at 13:05/mi → 3.25 × 785 = 2551s
```

## Pace to Power Mapping

For runners without power meters, paces are mapped to approximate FTP percentages:

| Pace Range | Power %FTP |
|------------|------------|
| 13:05+/mi  | 60%        |
| 11:55-13:05/mi | 70%    |
| 11:35-11:55/mi | 80%    |
| 10:30-11:35/mi | 95%    |
| <10:30/mi  | 100%       |

Intervals.icu converts these to pace targets based on your athlete profile settings.

## Building

```bash
go vet ./...
go test -race ./...
go build -o runna-sync .
```

## Limitations

- Workouts without descriptions are skipped
- Zone-based workouts (Z1, Z4, etc.) require athlete zones configured in Intervals.icu
- All workouts scheduled at 08:00 local time
- Power mapping is approximate; adjust `paceToPower()` for your fitness level

## License

MIT
