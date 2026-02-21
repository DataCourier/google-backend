# QuietBeat (quietbeat.app)

Formerly SafetyPulse. Passive safety monitoring backend. Beacons (phones carried by vulnerable family members) send periodic pings. Guardians monitor them via a dashboard.

Backend service name is still `safety-pulse` in Cloud Run and the codebase.

## Base URL

```
https://safety-pulse-133184350530.us-central1.run.app
```

## Auth

Every request (except `/health`) requires `Authorization` header.

Dev tokens: `local:<user-id>` or `local:<user-id>:<email>:<name>`

```
Authorization: local:user-123
Authorization: local:user-123:alice@example.com:Alice
```

## Endpoints

### Family

| Method | Path | Description | Success | Errors |
|--------|------|-------------|---------|--------|
| POST | `/family/` | Create family (guardian) | 201 | 400, 401 |
| POST | `/family/join` | Join family (beacon) | 200 | 400, 404, 409, 401 |
| GET | `/family/` | Guardian dashboard | 200 | 404, 401 |

### Beacon

| Method | Path | Description | Success | Errors |
|--------|------|-------------|---------|--------|
| POST | `/beacon/{id}/ping` | Send ping (never rejects valid JSON) | 201 | 404, 401 |
| GET | `/beacon/{id}/pings` | List pings (`?from=&to=` RFC3339) | 200 | 403, 404, 401 |
| GET | `/beacon/{id}/` | Get beacon details | 200 | 403, 404 |
| PUT | `/beacon/{id}/` | Update beacon (push_token, etc.) | 200 | 403, 404 |
| DELETE | `/beacon/{id}/` | Delete beacon (guardian or creator) | 200 | 403, 404 |

### Health

| Method | Path | Description | Success | Errors |
|--------|------|-------------|---------|--------|
| GET | `/health` | No auth needed | 200 | 503 |

## Request/Response Examples

### Create family
```bash
curl -X POST $BASE/family/ -H "Authorization: local:guardian-1" -H "Content-Type: application/json" \
  -d '{"family_name":"The Smiths"}'
# 201 → {"org_id":"uuid","family_name":"The Smiths","invite_token":"hex-token"}
```

### Join family
```bash
curl -X POST $BASE/family/join -H "Authorization: local:beacon-1" -H "Content-Type: application/json" \
  -d '{"invite_token":"TOKEN","device_name":"Grandma iPhone","device_model":"iPhone 15","os_version":"18.2"}'
# 200 → {"org_id":"uuid","beacon_id":"uuid","family_name":"The Smiths"}
```

### Send ping
```bash
curl -X POST $BASE/beacon/BEACON_ID/ping -H "Authorization: local:beacon-1" -H "Content-Type: application/json" \
  -d '{"battery_level":72,"charging_state":"unplugged","step_count":3456,"latitude":51.5074,"longitude":-0.1278,"ping_source":"widget_refresh","timestamp":"2026-02-21T14:30:00Z"}'
# 201 → {"id":"ping-uuid"}
```

All ping fields are optional. Partial pings are accepted and stored.

### Guardian dashboard
```bash
curl $BASE/family/ -H "Authorization: local:guardian-1"
# 200 → {"family_name":"The Smiths","org_id":"uuid","beacons":[{"device_name":"...","battery_level":72,...}]}
```

## Source Code

- `main.go` — server setup, health endpoint
- `family.go` — POST /family, POST /family/join, GET /family (dashboard)
- `beacon.go` — ping, list pings, get/update/delete beacon
- `auth/auth.go` — auth middleware (local tokens, session tokens, combined mode)
- `buckets/` — generic org-scoped CRUD (from game-service)
- `DESIGN.md` — full architecture, data model, pairing flow, reliability principles

## Running

```bash
# Local (emulator)
gcloud emulators firestore start --host-port=localhost:9090  # terminal 1
FIRESTORE_EMULATOR_HOST=localhost:9090 go run .              # terminal 2 → localhost:8083

# Tests (local)
./test.sh -run TestE2E -v

# Tests (production)
BASE_URL=https://safety-pulse-133184350530.us-central1.run.app go test -run TestE2E -v

# Deploy
./deploy.sh
```
