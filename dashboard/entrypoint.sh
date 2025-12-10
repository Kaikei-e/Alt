#!/bin/bash
set -e

# Start SSE server in background
echo "Starting SSE server on port ${SSE_PORT:-8000}..."
python sse_server.py &
SSE_PID=$!

# Function to handle shutdown signals
cleanup() {
    echo "Shutting down services..."
    kill $SSE_PID 2>/dev/null || true
    kill $STREAMLIT_PID 2>/dev/null || true
    wait $SSE_PID 2>/dev/null || true
    wait $STREAMLIT_PID 2>/dev/null || true
    exit 0
}

# Register signal handlers
trap cleanup SIGTERM SIGINT

# Wait a moment for SSE server to start
sleep 2

# Check if SSE server is running
if ! kill -0 $SSE_PID 2>/dev/null; then
    echo "ERROR: SSE server failed to start"
    exit 1
fi

echo "SSE server started (PID: $SSE_PID)"

# Start Streamlit in foreground
echo "Starting Streamlit app..."
streamlit run app.py --server.port=8501 --server.address=0.0.0.0 &
STREAMLIT_PID=$!

# Wait for Streamlit to start
sleep 2

# Check if Streamlit is running
if ! kill -0 $STREAMLIT_PID 2>/dev/null; then
    echo "ERROR: Streamlit failed to start"
    cleanup
    exit 1
fi

echo "Streamlit app started (PID: $STREAMLIT_PID)"

# Wait for either process to exit
wait -n

# If we get here, one of the processes exited
echo "One of the services exited unexpectedly"
cleanup

