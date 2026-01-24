#!/bin/bash
set -e

EMULATOR_PID_FILE="/tmp/firestore-emulator.pid"
EMULATOR_LOG="/tmp/firestore-emulator.log"
EMULATOR_HOST="localhost"
EMULATOR_PORT="9099"

start_emulator() {
    echo "Starting Firestore emulator..."
    gcloud emulators firestore start --host-port="${EMULATOR_HOST}:${EMULATOR_PORT}" > "$EMULATOR_LOG" 2>&1 &
    echo $! > "$EMULATOR_PID_FILE"

    # Wait for emulator to be ready
    for i in {1..30}; do
        if curl -s "http://${EMULATOR_HOST}:${EMULATOR_PORT}" > /dev/null 2>&1; then
            echo "Emulator ready on ${EMULATOR_HOST}:${EMULATOR_PORT}"
            return 0
        fi
        sleep 0.5
    done

    echo "Emulator failed to start. Log:"
    cat "$EMULATOR_LOG"
    exit 1
}

stop_emulator() {
    if [ -f "$EMULATOR_PID_FILE" ]; then
        PID=$(cat "$EMULATOR_PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Stopping emulator (PID $PID)..."
            kill "$PID" 2>/dev/null || true
            rm -f "$EMULATOR_PID_FILE"
        fi
    fi
}

# Check if emulator already running
if curl -s "http://${EMULATOR_HOST}:${EMULATOR_PORT}" > /dev/null 2>&1; then
    echo "Emulator already running."
    STARTED_EMULATOR=false
else
    start_emulator
    STARTED_EMULATOR=true
fi

# Run tests
export FIRESTORE_EMULATOR_HOST="${EMULATOR_HOST}:${EMULATOR_PORT}"
export PATH=$HOME/go/bin:/usr/local/go/bin:$PATH

echo "Running tests..."
go test -v "$@"
TEST_EXIT=$?

# Stop emulator if we started it
if [ "$STARTED_EMULATOR" = true ]; then
    stop_emulator
fi

exit $TEST_EXIT
