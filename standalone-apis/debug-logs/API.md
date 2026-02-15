# Debug Logs API

A lightweight remote debug log service. Any app can POST debug state and you can read it back via curl. Useful during development when you can't easily see device logs.

**Base URL:** `https://debug-logs-133184350530.us-central1.run.app`

## Quick Start

### From your app (Kotlin)
```kotlin
// Post debug state — call this whenever you want to inspect something
val client = OkHttpClient()
val json = """{"app":"my-app","logs":{"screen":"main","state":"loading","items":5}}"""
client.newCall(Request.Builder()
    .url("https://debug-logs-133184350530.us-central1.run.app/log")
    .post(json.toRequestBody("application/json".toMediaType()))
    .build()
).execute()
```

### From your terminal
```bash
# Read latest dump
curl -s https://debug-logs-133184350530.us-central1.run.app/log/my-app | python3 -m json.tool

# List all apps
curl -s https://debug-logs-133184350530.us-central1.run.app/logs | python3 -m json.tool
```

---

## Endpoints

### `GET /health`

Health check.

**Response:** `200 OK` — `ok`

---

### `POST /log`

Store debug data for an app. Overwrites any previous dump for that app name.

**Request:**
```json
{
  "app": "my-app",
  "logs": <any valid JSON>
}
```

`logs` can be a string, object, array — whatever you want to inspect.

**Response:** `200 OK`
```json
{"status": "ok"}
```

**Errors:**
- `400` — missing `app` field or invalid JSON

---

### `GET /log/{app}`

Retrieve the latest debug dump for an app.

**Response:** `200 OK`
```json
{
  "app": "my-app",
  "logs": { ... },
  "updated_at": 1771189322023
}
```

| Field | Type | Description |
|-------|------|-------------|
| `app` | string | App name |
| `logs` | any | Whatever was posted |
| `updated_at` | int64 | Unix millis when last posted |

**Errors:**
- `404` — no logs found for that app name

---

### `GET /logs`

List all apps that have debug logs (without the full log data).

**Response:** `200 OK`
```json
[
  {"app": "ticker-widget", "updated_at": 1771189322023},
  {"app": "field-notes", "updated_at": 1771189300000}
]
```

---

## Notes

- Each app gets one slot — posting overwrites the previous dump
- No auth required (debug builds only, don't ship to production)
- Data lives in Firestore `debug_logs` collection
- Service scales to zero when idle (no cost when not in use)
