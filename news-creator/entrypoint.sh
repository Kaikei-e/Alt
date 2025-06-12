#!/usr/bin/env bash

# Simple approach: just run ollama serve in the foreground
echo "Starting ollama server..."
exec ollama serve
