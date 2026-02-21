# QuietBeat (quietbeat.app) — Passive Safety Monitoring Backend

> App name: **QuietBeat** (quietbeat.app). Backend service name remains `safety-pulse` in Cloud Run and codebase.

A REST API that receives periodic pings from an iOS app and stores them for analysis. The app monitors phone activity signals (steps, battery, location) so family members can detect when an isolated person may be in trouble. Core principle: if the phone goes silent, something may be wrong.

## Goal of This Phase

Collect pings from a real iPhone carried for a month. Validate that iOS delivers enough background refresh opportunities to make passive monitoring reliable. Analyze the gaps.

## Guardian / Beacon Model

the api will have a guardian and a beacon. beacon sends pings, guardian has many beacons they can be taking care of. in future this could be done with caretakers so the beacons would belong to an org and guardians would be assigned to beacons. so two ways we could go here 1) build it all out using the org mode backend (perfect, we get to use it and improve it) or 2) we hack it as random endpoints and potentially miss an opportunity to test drive the org

most folks will use it as a direct beacon to guardian connection but will likely have a second person being looked after too. so at minimum, we're looking at a link table between the two. the org backend is not really making it more complicated because from the user's perspective we'll make it really simple. 1) open up your phone, setup yourself as guardian, tap "add family member", shows QR code. 2) second app select "This device is beign looked after" and scans a QR code. either way extremely simple pairing, org mode is just saying - you can add more guardians and the data is locked down to your family = org. so we already want org mode I think.

## How Org Mode Maps to SafetyPulse

**Org = Family circle.** When a guardian sets up the app, an org is created automatically. The guardian is the `owner`. When they scan a beacon's QR code to pair, the beacon's user account joins the org as a `member`.

**Roles:**
- `owner` / `admin` = **Guardian** — can see all beacons, their pings, location, battery. Can add/remove family members.
- `member` = **Beacon** — can post pings to the org. Can see their own data. Cannot see other beacons.

**Org buckets used:**
- `beacons` — one record per beacon device, owned by the org. Contains device info, push token, last_seen_at.
- `pings` — high-volume bucket. Each ping is an org record created by the beacon's user. Guardians can read all pings; beacons can only see their own (visibility: `private`, but guardians are `admin`+).

This means:
- All ping data is locked to the family org — hard boundary, no leaks
- Adding a second guardian = just adding another `admin` to the org
- Adding a second beacon = another `member` in the org
- No custom access control code needed — org mode handles it

## Architecture

```
standalone-apis/safety-pulse/
├── main.go          # Server setup, custom endpoints (ping, pairing)
├── auth/            # Copied from game-service/auth
├── buckets/         # Copied from game-service/buckets (org + personal)
├── org.json         # Bucket permissions config
├── go.mod
├── go.sum
└── DESIGN.md
```

Go backend with Chi router + Firestore. Copies the org bucket system from game-service. Deployed to Cloud Run.

**The framework has two backend modes:**

1. **Personal mode** — just you and your data. Everything is scoped to you, you have permission to do anything with your data, and you don't see anyone else's data.
2. **Org mode** — everything is tied to the org. Nothing enters or leaves the org from outside. The org is above its members. Members get permissions within the org, the owner is the ultimate admin, there are roles (owner > admin > manager > member > guest).

SafetyPulse uses **org mode** — the family is the org, and all beacon/ping data belongs to the family, not to individual users.

**Note:** The org backend in game-service is half-baked — it has the core structure (membership, roles, visibility, org-scoped CRUD) but hasn't been used in a real app yet. SafetyPulse is the first real consumer. As we build, we will finish baking the org system and write tests alongside every feature we use. This is a test-drive — expect to fix gaps, improve the org code, and backport improvements to game-service.

## Auth

Same auth system as game-service. For phase 1, local dev tokens (`local:user-a`). The iOS app will use Firebase Auth (Apple Sign In) — each user gets a `user_id`, and that's how they interact with the org.

## org.json — Bucket Permissions

```json
{
  "resources": {
    "beacons": {
      "write": "member",
      "default_visibility": "org-wide"
    },
    "pings": {
      "write": "member",
      "default_visibility": "private"
    }
  }
}
```

- `beacons`: org-wide visibility — all guardians and beacons can see all devices in the family
- `pings`: private visibility — only the beacon that created it can see it... BUT guardians are admin+ so they bypass visibility and see all pings

## Firestore Collections

### `org-beacons` (org bucket)

One record per beacon device. Created when a beacon pairs with a family.

```json
{
  "id": "beacon-uuid",
  "org_id": "family-abc",
  "created_by": "user-grandma",
  "device_name": "Grandma's iPhone",
  "device_model": "iPhone 15",
  "os_version": "18.2",
  "push_token": "",
  "last_seen_at": "2026-02-21T14:30:00Z",
  "visibility": "org-wide",
  "created_at": "2026-02-21T10:00:00Z",
  "updated_at": "2026-02-21T14:30:00Z"
}
```

### `org-pings` (org bucket)

One document per ping. High-volume.

```json
{
  "id": "ping-uuid",
  "org_id": "family-abc",
  "created_by": "user-grandma",
  "beacon_id": "beacon-uuid",
  "timestamp": "2026-02-21T14:30:00Z",
  "received_at": "2026-02-21T14:30:01Z",
  "battery_level": 72,
  "charging_state": "unplugged",
  "step_count": 3456,
  "latitude": 51.5074,
  "longitude": -0.1278,
  "ping_source": "widget_refresh",
  "visibility": "private",
  "created_at": "2026-02-21T14:30:01Z",
  "updated_at": "2026-02-21T14:30:01Z"
}
```

### `org-members` (framework)

Standard org membership. Created during pairing flow.

```json
{
  "id": "family-abc-user-grandma",
  "org_id": "family-abc",
  "user_id": "user-grandma",
  "role": "member",
  "invited_by": "user-michal",
  "joined_at": "2026-02-21T10:00:00Z"
}
```

## Endpoints

### Custom endpoints (SafetyPulse-specific)

All endpoints require `Authorization: Bearer <token>`. The server resolves the user's org from `org-members` automatically — clients never need to know org IDs.

#### `POST /family`

Guardian creates a new family (org).

```json
// Request
{ "family_name": "The Smiths" }

// Response: 201 Created
{ "org_id": "family-uuid", "invite_token": "random-token-for-qr" }
```

#### `POST /family/join`

Beacon joins a family by scanning QR code containing the invite token.

```json
// Request
{
  "invite_token": "random-token-for-qr",
  "device_name": "Grandma's iPhone",
  "device_model": "iPhone 15",
  "os_version": "18.2"
}

// Response: 200 OK
{ "org_id": "family-uuid", "beacon_id": "beacon-uuid", "family_name": "The Smiths" }
```

#### `GET /family`

Guardian dashboard. Returns all beacons with their latest ping.

```json
// Response: 200 OK
{
  "family_name": "The Smiths",
  "org_id": "family-uuid",
  "beacons": [
    {
      "id": "beacon-uuid",
      "device_name": "Grandma's iPhone",
      "last_seen_at": "2026-02-21T14:30:00Z",
      "battery_level": 72,
      "charging_state": "unplugged",
      "step_count": 3456,
      "latitude": 51.5074,
      "longitude": -0.1278,
      "ping_source": "widget_refresh",
      "minutes_since_last_ping": 12
    }
  ]
}
```

#### Beacon endpoints

```
POST   /beacon/{id}/ping              # Send a ping
GET    /beacon/{id}/pings             # List pings for this beacon
GET    /beacon/{id}/pings?from=&to=   # Filter by time range
GET    /beacon/{id}                   # Get beacon details
PUT    /beacon/{id}                   # Update beacon (push token, os_version, etc.)
DELETE /beacon/{id}                   # Remove beacon from family
```

#### `POST /beacon/{id}/ping`

The primary endpoint. Beacon posts its signals. **This endpoint never rejects a ping.** Whatever JSON the client sends, we save the entire raw payload. We extract known fields for indexing/querying, but the full payload is always preserved.

```json
// Request — any JSON body, but we expect these fields:
{
  "timestamp": "2026-02-21T14:30:00Z",
  "battery_level": 72,
  "charging_state": "unplugged",
  "step_count": 3456,
  "latitude": 51.5074,
  "longitude": -0.1278,
  "ping_source": "widget_refresh"
}

// Response: 201 Created
{ "id": "ping-uuid" }
```

**Storage model:** The ping document stores a `payload` field containing the entire raw JSON from the client. Known fields (`timestamp`, `battery_level`, etc.) are also extracted to top-level fields for querying. If a field is missing or malformed, it's just absent from the top level — the raw payload is still saved.

```json
// What gets written to org-pings:
{
  "id": "ping-uuid",
  "org_id": "family-uuid",
  "created_by": "user-grandma",
  "beacon_id": "beacon-uuid",
  "received_at": "2026-02-21T14:30:01Z",
  "timestamp": "2026-02-21T14:30:00Z",
  "battery_level": 72,
  "charging_state": "unplugged",
  "step_count": 3456,
  "latitude": 51.5074,
  "longitude": -0.1278,
  "ping_source": "widget_refresh",
  "payload": { ... raw JSON as-is ... },
  "visibility": "private",
  "created_at": "2026-02-21T14:30:01Z",
  "updated_at": "2026-02-21T14:30:01Z"
}
```

Server-side: resolves user → org, verifies beacon belongs to org, saves payload + extracted fields to `org-pings`, updates `last_seen_at` on the beacon.

#### `GET /beacon/{id}/pings`

List pings for a beacon. Guardian (admin+) can see any beacon's pings. Beacon (member) can only see their own.

Query params:
- `from` — RFC3339, filter pings after this time
- `to` — RFC3339, filter pings before this time
- `limit` — max results (default 100, max 1000)

```json
// Response: 200 OK
{ "pings": [...], "count": 42 }
```

#### `GET /health`

```json
// Response: 200 OK
{ "status": "ok" }
```

## Pairing Flow (User Experience)

### Guardian setup
1. Open app → "I'm a Guardian" → Sign in with Apple
2. App calls `POST /family` → gets `org_id` + `invite_token`
3. App shows QR code containing `invite_token`
4. (Can regenerate QR / share link anytime to add more beacons)

### Beacon setup
1. Open app → "This device is being looked after" → Sign in with Apple
2. Scan guardian's QR code → extracts `invite_token`
3. App calls `POST /family/join` with token + device info
4. App starts sending pings to `POST /beacon/{id}/ping`
5. Done — widget installed, pings flowing

### Adding a second guardian
1. Existing guardian taps "Add Guardian" → shares invite link
2. New guardian opens link → Sign in with Apple
3. App calls `POST /family/join` but server adds them as `admin` instead of `member`
4. (Needs a flag in the invite or a separate endpoint — TBD)

## Running Locally

```bash
# Terminal 1: Firestore emulator
gcloud emulators firestore start --host-port=localhost:9090

# Terminal 2: Go backend
cd standalone-apis/safety-pulse
FIRESTORE_EMULATOR_HOST=localhost:9090 go run .
# → http://localhost:8083
```

Port: **8083**.

## Reliability Principles

This API must never fail and — more importantly — never fail silently. A missed ping could look like a person in danger, so the backend must be bulletproof.

### Review steps (during implementation)

- Every endpoint returns a clear, structured error response — never a bare 500 or empty body
- `POST /beacon/{id}/ping` must be as defensive as possible: validate input, but accept partial data (e.g. if location is missing, store the ping anyway with zeroed lat/lng — a partial ping is better than no ping)
- All Firestore writes must check for errors and return them to the client so the app can retry
- Log every error with enough context to diagnose (beacon ID, org ID, what failed)
- `GET /health` should verify Firestore connectivity, not just return a static 200
- Panic recovery middleware (Chi's `middleware.Recoverer`) so a panic in one request doesn't crash the server

### Additional setup

- **Error tracking**: Set up Sentry (or GCP Error Reporting) — every unhandled error and every 5xx response must be captured and alerted on
- **Structured logging**: Use GCP-compatible structured JSON logs so errors are searchable in Cloud Logging
- **Uptime monitoring**: Set up an external ping to `/health` (e.g. GCP Uptime Check or UptimeRobot) — if the backend goes down, we need to know before the app notices
- **Request logging**: Log every ping received (timestamp, beacon ID, source) so we can cross-reference with gaps in the data

## Phase 2 (follow-up after data evaluation)

- **Silent push**: Store APNs push tokens in beacon registration. Add `POST /push/{beacon_id}` to trigger a silent push. Determine optimal timing — send push when no ping received in X hours.
- **Alerting**: Detect prolonged silence, notify guardians via push notification.
- **Multiple guardians**: Flesh out invite flow for adding guardians (admin role) vs beacons (member role).
- **Staging + prod rename**: Nuke the current `safety-pulse` Cloud Run service. Create two properly named services: `quietbeat-staging` and `quietbeat-prod`. Run e2e tests against staging before deploying to prod. Update `deploy.sh` and Makefile targets accordingly.
- **Web join page**: Deploy a static HTML page to Cloudflare (custom domain) that lets a beacon join a family via a shared link instead of QR code scanning. Guardian sends grandma a URL like `https://quietbeat.app/join?token=abc123`, she opens it on her phone, taps join, done. Needs a lightweight web endpoint or the page calls `POST /family/join` directly with the token from the URL. Much simpler onboarding for non-technical users — no QR scanning, no app-to-app pairing.

## Build Steps

1. **Copy org backend from game-service** — copy `auth/` and `buckets/` packages, `org.json`. Adapt imports, strip what we don't need.
2. **Set up Go module** — `go.mod` with dependencies (Chi, Firestore, UUID).
3. **Write main.go skeleton** — Chi router, Firestore client, middleware, health endpoint. Verify it boots.
4. **Implement `POST /family` and `POST /family/join`** — org creation, invite tokens, pairing flow. Write tests.
5. **Implement `POST /beacon/{id}/ping`** — the core endpoint. Raw payload storage + field extraction. Write tests.
6. **Implement `GET /beacon/{id}/pings`** — list pings with time range filtering. Write tests.
7. **Implement `GET /family`** — guardian dashboard, latest ping per beacon. Write tests.
8. **Implement beacon CRUD** — `GET/PUT/DELETE /beacon/{id}`. Write tests.
9. **Reliability pass** — structured logging, error responses, health check with Firestore connectivity.
10. **Add Makefile deploy target** — `deploy-safety` in root Makefile.
11. **Test end-to-end** — Firestore emulator, full pairing + ping flow via curl.

## Non-Goals (this phase)

- Alerting logic
- Silent push sending
- CloudKit / Android
- Frontend / web UI
