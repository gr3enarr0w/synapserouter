# Unraid Setup: runna-sync

## Quick Start

1. Copy the image to your Unraid server:
```bash
# Build on your Mac
cd runna-sync-test
podman build -t runna-sync .
podman save runna-sync | gzip > runna-sync.tar.gz

# Transfer to Unraid
scp runna-sync.tar.gz root@unraid:/tmp/

# On Unraid
docker load < /tmp/runna-sync.tar.gz
```

2. Create env file on Unraid:
```bash
mkdir -p /mnt/user/appdata/runna-sync
cat > /mnt/user/appdata/runna-sync/.env << 'EOF'
RUNNA_ICAL_URL=https://cal.runna.com/da8a632a9763fe4b9e0434ed1ace5a95.ics
INTERVALS_ATHLETE_ID=i247251
INTERVALS_API_TOKEN=6muslqlak3bt7zncc1wyratld
ATHLETE_WEIGHT_KG=72.6
TARGET_5K_MINUTES=35
SYNC_DAYS=14
POLL_INTERVAL_MINUTES=60
EOF
```

3. Run the container:
```bash
docker run -d \
  --name runna-sync \
  --restart unless-stopped \
  --env-file /mnt/user/appdata/runna-sync/.env \
  runna-sync
```

## That's it

The container runs continuously:
- Polls Runna iCal feed every hour
- Only syncs when workouts change
- Updates existing workouts in place (never duplicates)
- Auto-updates FTP from pace data every Sunday at 6am
- Pushes pace + power targets to Intervals.icu → Garmin

## Logs

```bash
docker logs runna-sync
docker logs -f runna-sync  # follow
```

## Manual Commands

```bash
# Force a sync now
docker exec runna-sync /runna-sync once

# Update FTP from last 30 days of runs
docker exec runna-sync /runna-sync update-pace 30

# Update FTP from Stryd power data
docker exec runna-sync /runna-sync update-power 30
```

## Unraid Community Applications (optional)

To add via Unraid's Docker tab manually:
- **Repository:** `runna-sync` (local image)
- **Network:** bridge
- **Extra Parameters:** `--restart unless-stopped`
- **Post Arguments:** (leave empty — defaults to continuous sync)
- Add env vars from the list above

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `RUNNA_ICAL_URL` | Yes | — | Your Runna iCal feed URL |
| `INTERVALS_ATHLETE_ID` | Yes | — | Intervals.icu athlete ID (e.g., i247251) |
| `INTERVALS_API_TOKEN` | Yes | — | API token from Intervals.icu Developer Settings |
| `ATHLETE_WEIGHT_KG` | No | 72.6 | Your weight in kg |
| `TARGET_5K_MINUTES` | No | 35 | Target 5K time for FTP calculation |
| `SYNC_DAYS` | No | 14 | How many days ahead to sync |
| `POLL_INTERVAL_MINUTES` | No | 60 | How often to check for changes |

## Image Size

6 MB total (scratch base, no OS).
