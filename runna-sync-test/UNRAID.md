# Unraid Setup: runna-sync

## Install as Unraid App

### Step 1: Load the image

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

### Step 2: Install the app template

```bash
# On Unraid — copy the template to Community Apps private folder
mkdir -p /boot/config/plugins/community.applications/private/runna-sync
```

Copy `unraid-template.xml` to that folder as `runna-sync.xml`:
```bash
scp unraid-template.xml root@unraid:/boot/config/plugins/community.applications/private/runna-sync/runna-sync.xml
```

### Step 3: Install from Unraid UI

1. Open Unraid web UI
2. Go to **Apps** tab
3. Click **Private** on the left sidebar
4. You'll see **runna-sync** — click **Install**
5. Fill in your settings:
   - **Runna iCal URL**: Your feed URL from Runna app settings
   - **Intervals.icu Athlete ID**: Your ID (e.g., `i247251`)
   - **Intervals.icu API Token**: From Intervals.icu Settings → Developer Settings
   - **Weight**: Your weight in kg
   - **Target 5K**: Your 5K target time in minutes
6. Click **Apply**

The app starts automatically and runs continuously.

## What It Does

- Polls Runna iCal feed every hour (configurable)
- Only syncs when workouts change
- Updates existing workouts in place (never creates duplicates)
- Pushes pace (sec/km) + power (watts) targets to each workout step
- Auto-updates FTP from your pace data every Sunday at 6am
- Intervals.icu auto-pushes to Garmin Connect → your watch

## Logs

In Unraid UI: click the runna-sync icon → **Logs**

Or via terminal:
```bash
docker logs -f runna-sync
```

## Manual Commands

Click the runna-sync icon → **Console**, then:
```bash
# Force a sync now
/runna-sync once

# Update FTP from last 30 days of runs
/runna-sync update-pace 30

# Update FTP from Stryd power data
/runna-sync update-power 30
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `RUNNA_ICAL_URL` | Yes | — | Your Runna iCal feed URL |
| `INTERVALS_ATHLETE_ID` | Yes | — | Intervals.icu athlete ID |
| `INTERVALS_API_TOKEN` | Yes | — | API token (masked in UI) |
| `ATHLETE_WEIGHT_KG` | No | 72.6 | Weight in kg |
| `TARGET_5K_MINUTES` | No | 35 | Target 5K time for FTP |
| `SYNC_DAYS` | No | 14 | Days ahead to sync |
| `POLL_INTERVAL_MINUTES` | No | 60 | Poll frequency in minutes |

## Image Size

6 MB (scratch base, static Go binary + CA certs only).
