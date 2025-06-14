#!/bin/bash
# This script starts opencloud and opencloud-sftp in the background inside a Docker container to then run end-to-end tests.
set -e  # Exit on any error

# Function to handle cleanup on exit
cleanup() {
    echo "Cleaning up background processes..."
    # Kill background processes if they're still running
    if [[ -n $PID1 ]] && kill -0 $PID1 2>/dev/null; then
        echo "Stopping process 1 (PID: $PID1)"
        kill $PID1
    fi
    if [[ -n $PID2 ]] && kill -0 $PID2 2>/dev/null; then
        echo "Stopping process 2 (PID: $PID2)"
        kill $PID2
    fi
    if [[ -n $PID3 ]] && kill -0 $PID3 2>/dev/null; then
            echo "Stopping process 3 (PID: $PID3)"
            kill $PID3
        fi
    exit 0
}

# Set up signal handlers for cleanup
trap cleanup SIGTERM SIGINT

echo "Starting opencloud..."
OC_MACHINE_AUTH_API_KEY='3RIWu=LE5kNUqDHM0xH*Dxe=U9sczrwY' /usr/local/bin/opencloud server &
PID1=$!
echo "opencloud started with PID: $PID1"
sleep 5

echo "Starting opencloud-sftp..."
OC_MACHINE_AUTH_API_KEY='3RIWu=LE5kNUqDHM0xH*Dxe=U9sczrwY' /usr/local/bin/opencloud-sftp server &
PID2=$!
echo "opencloud-sftp started with PID: $PID2"
sleep 5

# Check if background processes are still running
if ! kill -0 $PID1 2>/dev/null; then
    echo "Error: Failed to start opencloud. Exited early?"
    exit 1
fi

if ! kill -0 $PID2 2>/dev/null; then
    echo "Error: Failed to start opencloud-sftp. Exited early?"
    exit 1
fi

make test-e2e
