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

# Ensure base model exists
echo "Ensuring gpt-oss:20b model is available..."
if ! ollama list 2>/dev/null | grep -q "gpt-oss:20b"; then
  echo "Pulling gpt-oss:20b model (this will take a while)..."
  if ollama pull gpt-oss:20b; then
      echo "  Model gpt-oss:20b pulled successfully"
  else
      echo "  Error: Failed to pull gpt-oss:20b"
      # Don't exit here, maybe the custom model exists or we are offline
  fi
else
  echo "  Model gpt-oss:20b already exists"
fi

# Create custom CPU model
echo "Creating custom model gpt-oss20b-cpu..."
echo "Creating/Updating custom model gpt-oss20b-cpu..."
# Always try to create/update to capture Modelfile changes
if [ -f "/home/ollama-user/Modelfile.gpt-oss20b-cpu" ]; then
  if ollama create gpt-oss20b-cpu -f /home/ollama-user/Modelfile.gpt-oss20b-cpu; then
    echo "  Model gpt-oss20b-cpu created/updated successfully"
  else
    echo "  Error: Failed to create gpt-oss20b-cpu"
  fi
else
  echo "  Error: Modelfile not found"
fi

# Preload model to warm up
echo "Preloading model..."
curl -s http://127.0.0.1:11434/api/chat \
  -d '{"model":"gpt-oss20b-cpu","keep_alive":-1}' >/dev/null

# Wait for server process
wait "$SERVER_PID"
