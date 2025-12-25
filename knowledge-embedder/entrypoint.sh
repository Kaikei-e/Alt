#!/usr/bin/env bash
set -euo pipefail

echo "Starting Ollama server..."

# Start Ollama in background
ollama serve &
SERVER_PID=$!

# Wait for Ollama to be ready
echo "Waiting for Ollama server to start..."
for i in $(seq 1 60); do
  if curl -fs "http://127.0.0.1:11434/api/tags" >/dev/null 2>&1; then
    echo "  Server is up after $i seconds"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

if ! curl -fs "http://127.0.0.1:11434/api/tags" >/dev/null 2>&1; then
  echo "Error: Ollama server did not start in time"
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

# Ensure embedding model exists
echo "Ensuring embeddinggemma model is available..."
if ! ollama list 2>/dev/null | grep -q "embeddinggemma"; then
  echo "Pulling embeddinggemma model..."
  if ollama pull embeddinggemma; then
      echo "  Model embeddinggemma pulled successfully"
  else
      echo "  Error: Failed to pull embeddinggemma"
  fi
else
  echo "  Model embeddinggemma already exists"
fi

# Wait for server process
wait "$SERVER_PID"
