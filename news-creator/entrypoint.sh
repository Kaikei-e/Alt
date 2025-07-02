#!/usr/bin/env bash
set -euo pipefail

# Suppress verbose llama.cpp logs
export OLLAMA_LOG_LEVEL=ERROR
export LLAMA_LOG_LEVEL=0
export LLAMA_LOG_VERBOSITY=0

# Start ollama server in background with log suppression
ollama serve 2>/dev/null &
SERVER_PID=$!

# Wait for server to be ready
echo "Waiting for ollama server to start..."
until curl -s http://localhost:11434/api/tags >/dev/null 2>&1; do
    sleep 1
done

# Load the model to ensure it's available (with full log suppression)
echo "Loading phi4-mini:3.8b model (quiet)..."
# Suppress all output during model loading to avoid verbose llama_model_loader logs
OLLAMA_LOG_LEVEL=ERROR ollama run phi4-mini:3.8b >/dev/null 2>&1 << 'EOF'
exit
EOF

# Stop background server
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null || true

# Start server in foreground with log filtering
echo "Starting ollama server in foreground..."
# Filter out llama_model_loader messages **and** generic INFO-level lines (method #1 in the notes)
exec ollama serve 2>&1 | grep -vE "INFO|print_info:|llama_model_load|load_tensors:|llama_context:|ggml_cuda_init:" || true