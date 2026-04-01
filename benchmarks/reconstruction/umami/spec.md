# umami — Reconstruction Spec

## Overview

A privacy-focused, open-source web analytics platform — a modern alternative to Google Analytics. Tracks pageviews, custom events, and sessions without cookies. Built with Next.js, PostgreSQL, and an optional ClickHouse analytical backend. This spec covers the core analytics tracking API and stats query layer.

## Scope

**IN SCOPE:**
- Event collection API (POST /api/send)
- Session management (deterministic UUID from IP + UA + salt)
- Client tracking script (pageview + custom event tracking)
- Database schema (PostgreSQL via Prisma)
- Stats query API (website stats, pageview metrics, session metrics)
- Bot detection and IP filtering
- URL parsing (UTM parameters, click IDs)
- Batch collection endpoint (POST /api/batch)

**OUT OF SCOPE:**
- Full React dashboard UI (charts, tables, components)
- ClickHouse backend and Kafka streaming
- Redis caching layer
- Authentication system (login, JWT, teams)
- Admin panel (user management, website settings)
- Reports, segments, saved filters
- Link tracking and pixel tracking
- Revenue tracking
- Docker/CI configuration
- i18n/localization

**TARGET:** ~1000-1500 LOC TypeScript, ~15 source files

## Architecture

- **Language/Runtime:** TypeScript, Node.js 20+
- **Framework:** Next.js 15 (App Router)
- **Database:** PostgreSQL (via Prisma ORM)
- **Key dependencies:**
  - next (framework)
  - prisma / @prisma/client (ORM)
  - zod (request validation)
  - isbot (bot detection)
  - ua-parser-js (user agent parsing)
  - uuid (deterministic session IDs)
  - date-fns (date manipulation)
  - jsonwebtoken (session caching in JWT)

### Directory Structure
```
umami/
  package.json
  prisma/
    schema.prisma
  src/
    app/
      api/
        send/
          route.ts          # POST /api/send — main collection endpoint
        batch/
          route.ts          # POST /api/batch — batch collection
        websites/
          [websiteId]/
            stats/
              route.ts      # GET — website overview stats
            pageviews/
              route.ts      # GET — pageview metrics
            sessions/
              route.ts      # GET — session metrics
            active/
              route.ts      # GET — realtime active visitors
    lib/
      detect.ts             # Client info extraction (IP, UA, geo)
      crypto.ts             # Deterministic UUID generation
      constants.ts          # Event types, field limits, enums
      request.ts            # Request parsing and validation
      prisma.ts             # Prisma client + query helpers
      session.ts            # Session/visit ID management
    queries/
      sql/
        events/
          saveEvent.ts      # Insert website_event + event_data
        sessions/
          createSession.ts  # Upsert session record
          saveSessionData.ts
        getWebsiteStats.ts  # Aggregate stats query
        getPageviewMetrics.ts
        getSessionMetrics.ts
    tracker/
      index.ts              # Client-side tracking script
  .env
```

### Design Patterns
- **Stateless collection:** Each event request is self-contained (no server-side session state)
- **Deterministic session IDs:** UUID derived from IP + UserAgent + monthly salt (no cookies)
- **Visit windows:** Visit ID rotates hourly (30-min inactivity = new visit)
- **Denormalized events:** Each event stores full context (browser, OS, geo, UTM)

## Data Flow

### Event Collection Pipeline
```
Browser (tracker script)
  |
  | POST /api/send
  | { type: "event", payload: { website, url, referrer, title, screen, language } }
  |
  v
route.ts: POST handler
  |
  ├── 1. Validate with Zod schema
  ├── 2. Bot detection (isbot library)
  ├── 3. IP blocking check (IGNORE_IP env var, CIDR matching)
  |
  v
detect.ts: getClientInfo()
  ├── Extract IP from headers (x-forwarded-for, x-real-ip, cf-connecting-ip)
  ├── Parse User-Agent → browser, OS, device (ua-parser-js)
  ├── Geo-location from IP or CDN headers (cf-ipcountry, x-vercel-ip-country)
  ├── Parse screen dimensions from "WIDTHxHEIGHT" string
  |
  v
session.ts: getSession()
  ├── sessionId = uuid(websiteId + ip + userAgent + monthlySalt)
  ├── visitId = uuid(sessionId + hourlySalt)
  ├── Create Session record if new (INSERT ... ON CONFLICT DO NOTHING)
  |
  v
URL Parsing
  ├── Extract path, query, hostname from URL
  ├── Extract UTM params: utm_source, utm_medium, utm_campaign, utm_content, utm_term
  ├── Extract click IDs: gclid, fbclid, msclkid, ttclid, li_fat_id, twclid
  ├── Parse referrer → referrerPath, referrerDomain
  |
  v
saveEvent()
  ├── INSERT INTO website_event (all fields, truncated to max lengths)
  ├── If custom event data: INSERT INTO event_data (flattened key-value pairs)
  ├── ON CONFLICT DO NOTHING (idempotent)
  |
  v
Response: { cache: "JWT_TOKEN", sessionId, visitId }
```

### Stats Query Pipeline
```
Dashboard Request
  |
  | GET /api/websites/{id}/stats?startAt=...&endAt=...
  |
  v
getWebsiteStats()
  |
  SELECT
    COUNT(DISTINCT session_id) as visitors,
    COUNT(DISTINCT visit_id) as visits,
    SUM(event_count) as pageviews,
    SUM(CASE WHEN event_count = 1 THEN 1 ELSE 0 END) as bounces,
    SUM(max_time - min_time) as totaltime
  FROM (
    SELECT session_id, visit_id, COUNT(*) as event_count,
           MIN(created_at) as min_time, MAX(created_at) as max_time
    FROM website_event
    WHERE website_id = ? AND created_at BETWEEN ? AND ?
      AND event_type = 1  -- pageviews only
    GROUP BY session_id, visit_id
  )
```

## Core Components

### Database Schema (prisma/schema.prisma)

#### Website
```prisma
model Website {
  id        String   @id @default(uuid()) @db.Uuid
  name      String   @db.VarChar(100)
  domain    String?  @db.VarChar(500)
  shareId   String?  @unique @db.VarChar(50)
  createdAt DateTime @default(now()) @map("created_at")
  updatedAt DateTime @updatedAt @map("updated_at")
  deletedAt DateTime? @map("deleted_at")
  resetAt   DateTime? @map("reset_at")

  sessions      Session[]
  websiteEvents WebsiteEvent[]
  @@map("website")
}
```

#### Session
```prisma
model Session {
  id         String   @id @map("session_id") @db.Uuid
  websiteId  String   @map("website_id") @db.Uuid
  browser    String?  @db.VarChar(20)
  os         String?  @db.VarChar(20)
  device     String?  @db.VarChar(20)
  screen     String?  @db.VarChar(11)
  language   String?  @db.VarChar(35)
  country    String?  @db.Char(2)
  region     String?  @db.VarChar(100)
  city       String?  @db.VarChar(100)
  distinctId String?  @map("distinct_id") @db.VarChar(200)
  createdAt  DateTime @default(now()) @map("created_at")

  website    Website  @relation(fields: [websiteId], references: [id])
  events     WebsiteEvent[]
  @@index([websiteId, createdAt])
  @@map("session")
}
```

#### WebsiteEvent
```prisma
model WebsiteEvent {
  id             String   @id @default(uuid()) @db.Uuid
  websiteId      String   @map("website_id") @db.Uuid
  sessionId      String   @map("session_id") @db.Uuid
  visitId        String   @map("visit_id") @db.Uuid
  createdAt      DateTime @default(now()) @map("created_at")
  urlPath        String   @map("url_path") @db.VarChar(500)
  urlQuery       String?  @map("url_query") @db.VarChar(500)
  pageTitle      String?  @map("page_title") @db.VarChar(500)
  hostname       String?  @db.VarChar(100)
  referrerPath   String?  @map("referrer_path") @db.VarChar(500)
  referrerQuery  String?  @map("referrer_query") @db.VarChar(500)
  referrerDomain String?  @map("referrer_domain") @db.VarChar(500)
  eventType      Int      @default(1) @map("event_type")
  eventName      String?  @map("event_name") @db.VarChar(50)
  tag            String?  @db.VarChar(50)

  // UTM parameters
  utmSource      String?  @map("utm_source") @db.VarChar(500)
  utmMedium      String?  @map("utm_medium") @db.VarChar(500)
  utmCampaign    String?  @map("utm_campaign") @db.VarChar(500)
  utmContent     String?  @map("utm_content") @db.VarChar(500)
  utmTerm        String?  @map("utm_term") @db.VarChar(500)

  website  Website  @relation(fields: [websiteId], references: [id])
  session  Session  @relation(fields: [sessionId], references: [id])
  eventData EventData[]

  @@index([websiteId, createdAt])
  @@index([sessionId, createdAt])
  @@map("website_event")
}
```

#### EventData
```prisma
model EventData {
  id             String   @id @default(uuid()) @db.Uuid
  websiteId      String   @map("website_id") @db.Uuid
  websiteEventId String   @map("website_event_id") @db.Uuid
  dataKey        String   @map("data_key") @db.VarChar(500)
  stringValue    String?  @map("string_value") @db.VarChar(500)
  numberValue    Decimal? @map("number_value") @db.Decimal(19, 4)
  dateValue      DateTime? @map("date_value")
  dataType       Int      @map("data_type")
  createdAt      DateTime @default(now()) @map("created_at")

  websiteEvent WebsiteEvent @relation(fields: [websiteEventId], references: [id])
  @@index([websiteId, createdAt, dataKey])
  @@map("event_data")
}
```

### Event Types (src/lib/constants.ts)
```typescript
export const EVENT_TYPE = {
  pageView: 1,
  customEvent: 2,
  linkEvent: 3,
  pixelEvent: 4,
} as const;

export const DATA_TYPE = {
  string: 1,
  number: 2,
  boolean: 3,
  date: 4,
} as const;

export const URL_LENGTH = 500;
export const PAGE_TITLE_LENGTH = 500;
export const EVENT_NAME_LENGTH = 50;
```

### Crypto (src/lib/crypto.ts)
- **Purpose:** Deterministic UUID generation for sessions
- **API:**
  ```typescript
  function uuid(values: string[]): string
  // Creates UUID v5 from concatenated values
  // Used: uuid(websiteId, ip, userAgent, salt)

  function hash(value: string): string
  // SHA256 hash, hex encoded

  function secret(): string
  // Returns APP_SECRET env var or default
  ```
- **Session salt:** Hash of current month start (rotates monthly)
- **Visit salt:** Hash of current hour start (rotates hourly)

### Detect (src/lib/detect.ts)
- **Purpose:** Extract client info from request
- **API:**
  ```typescript
  interface ClientInfo {
    ip: string;
    userAgent: string;
    browser: string;
    os: string;
    device: string;
    screen: string;
    language: string;
    country: string;
    region: string;
    city: string;
  }

  function getClientInfo(request: Request, payload: any): ClientInfo
  ```
- **IP extraction order:** x-forwarded-for → x-real-ip → cf-connecting-ip → request socket
- **Geo extraction:** CDN headers (cf-ipcountry, x-vercel-ip-country) or MaxMind DB
- **UA parsing:** ua-parser-js → browser name, OS name, device type

### Tracker Script (src/tracker/index.ts)
- **Purpose:** Client-side script embedded on tracked websites
- **Initialization:** `<script src="/script.js" data-website-id="UUID">`
- **Auto-tracking:**
  - Fires pageview on page load
  - Hooks `history.pushState` and `history.replaceState` for SPA navigation
  - Listens for `data-umami-event` attributes on clicked elements
- **Manual API:**
  ```typescript
  umami.track(eventName?: string, eventData?: object): Promise<void>
  umami.identify(sessionData: object): Promise<void>
  ```
- **Privacy:** Respects Do Not Track, localStorage disable flag, domain filtering

### Stats Queries (src/queries/)
- **getWebsiteStats:** Visitors, visits, pageviews, bounces, total time
- **getPageviewMetrics:** Group by URL path, title, or referrer domain
- **getSessionMetrics:** Group by browser, OS, device, country, region, city
- **getActiveVisitors:** Count distinct sessions in last 5 minutes

## Configuration

### Environment Variables
```bash
DATABASE_URL=postgresql://user:pass@host:5432/umami
APP_SECRET=random-secret-string        # used for session salt
IGNORE_IP=127.0.0.1,192.168.0.0/16    # CIDR/IP blocklist
DISABLE_BOT_CHECK=false
REMOVE_TRAILING_SLASH=true
COLLECT_API_ENDPOINT=/api/send
```

## Test Cases

### Functional Tests
1. **Track pageview:** POST /api/send with type=event, url="/about" -> 200, creates website_event with eventType=1
2. **Track custom event:** POST /api/send with name="signup", data={plan:"pro"} -> creates event + event_data rows
3. **Session continuity:** Same IP+UA within same month -> same sessionId returned
4. **New visit:** Same session after 1 hour -> different visitId
5. **Bot filtering:** POST with bot User-Agent -> rejected (no event created)
6. **IP blocking:** POST from blocked IP -> rejected
7. **UTM extraction:** URL with ?utm_source=google -> stored in utmSource field
8. **Batch collection:** POST /api/batch with 3 events -> all 3 processed
9. **Website stats:** GET /api/websites/{id}/stats -> returns visitors, visits, pageviews, bounces
10. **Active visitors:** GET /api/websites/{id}/active -> count of sessions in last 5 min

### Edge Cases
1. **Long URL:** URL > 500 chars -> truncated to 500
2. **Missing fields:** POST with minimal payload (just website + url) -> works with defaults
3. **Duplicate event:** Same event sent twice -> ON CONFLICT DO NOTHING (idempotent)
4. **Invalid website ID:** POST with non-existent website -> 404

## Build & Run

### Setup
```bash
npm install
npx prisma generate
npx prisma db push
```

### Run
```bash
npm run dev
# Starts on http://localhost:3000
```

### Test
```bash
npm test
```

## Acceptance Criteria

1. Project builds with `npm run build` without errors
2. Prisma schema generates client and migrations apply cleanly
3. POST /api/send creates session + website_event records in PostgreSQL
4. Session IDs are deterministic (same IP+UA+month = same session)
5. Visit IDs rotate hourly
6. Bot user agents are detected and rejected
7. IP blocking works with CIDR notation
8. UTM parameters extracted from URLs and stored
9. Event data (custom properties) flattened and stored with correct data types
10. GET /api/websites/{id}/stats returns correct aggregate metrics
11. Tracker script sends pageview on load and SPA navigation
12. Batch endpoint processes multiple events in one request
13. Field length limits enforced (URL 500, event name 50)
14. ON CONFLICT DO NOTHING prevents duplicate events
