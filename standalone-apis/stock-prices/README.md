# Stock Prices Service

Fetches stock/crypto prices from Finnhub, writes static JSON files to GCS.

## Local Development

### Prerequisites

- Firestore emulator on port 9090
- fake-gcs-server on port 4443

```bash
# Start emulators (if not already running)
gcloud emulators firestore start --host-port=localhost:9090 &
docker start fake-gcs || docker run -d -p 4443:4443 --name fake-gcs fsouza/fake-gcs-server -scheme http

# Create the GCS bucket in fake-gcs
curl -s -X POST "http://localhost:4443/storage/v1/b?project=michal-playground-2026" \
  -H "Content-Type: application/json" -d '{"name": "stock-prices-dev"}'
```

### Run the service

```bash
export FIRESTORE_EMULATOR_HOST=localhost:9090
export STORAGE_EMULATOR_HOST=http://localhost:4443
export FINNHUB_API_KEY=<your-key>
export GCS_BUCKET=stock-prices-dev
go run .
# Listens on port 8081
```

### Run E2E tests

```bash
export BASE_URL=http://localhost:8081
export STORAGE_EMULATOR_HOST=http://localhost:4443
export GCS_BUCKET=stock-prices-dev
go test -v -run E2E -count=1 ./...
```

### Test against remote

```bash
export BASE_URL=https://stock-prices-xxx.run.app
export GCS_BUCKET=stock-prices-prod
go test -v -run E2E -count=1 ./...
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /heartbeat | `{"symbols": ["AAPL"]}` — increment subscriber count |
| POST | /fetch | Trigger a fetch cycle (called by Cloud Scheduler) |
| GET | /health | Health check |

## Port Map

| Service | Port |
|---------|------|
| Stock prices service | 8081 |
| Firestore emulator | 9090 |
| fake-gcs-server | 4443 |
| game-service | 8080 |
