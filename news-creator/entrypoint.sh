#!/usr/bin/env bash

# Simple approach: just run ollama serve in the foreground
echo "Starting ollama server..."

# Set environment variables for ollama
export OLLAMA_MODELS=/home/appuser/.ollama/models
export OLLAMA_HOST=0.0.0.0:11434

# Check if models directory exists and has content
if [ -d "$OLLAMA_MODELS" ] && [ "$(ls -A $OLLAMA_MODELS)" ]; then
    echo "Found models in $OLLAMA_MODELS"
    ls -la $OLLAMA_MODELS
else
    echo "Warning: No models found in $OLLAMA_MODELS"
fi

exec ollama serve
