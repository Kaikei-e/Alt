#!/usr/bin/env bash
set -euo pipefail

# Start ollama server in background
ollama serve &
SERVER_PID=$!

# Reduce Ollama log verbosity
export OLLAMA_LOG_LEVEL=ERROR

# Wait for server to be ready
echo "Waiting for ollama server to start..."
until curl -s http://localhost:11434/api/tags >/dev/null 2>&1; do
    sleep 1
done

# Load the model to ensure it's available
echo "Loading phi4-mini:3.8b model (quiet)..."
# Load model quietly to warm cache without flooding logs
ollama run phi4-mini:3.8b > /dev/null 2>&1

# Stop background server
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null

# Start server in foreground
echo "Starting ollama server in foreground..."
exec ollama serve