#!/usr/bin/env bash
# stop.sh - Stop the gateway server running on the configured port

PORT="${1:-8080}"

PID=$(lsof -ti tcp:"$PORT" 2>/dev/null)

if [ -z "$PID" ]; then
    echo "No process found on port $PORT"
    exit 0
fi

kill -TERM "$PID"
echo "Stopped PID $PID on port $PORT"
