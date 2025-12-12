#!/bin/bash
set -e

# Configuration
SOURCE_GOLDEN="./recap-subworker/data/golden_classification.json"
HOST_DATA_DIR="$(pwd)/recap-subworker/data"
TRAINING_IMAGE="alt-recap-training"
SUBWORKER_CONTAINER="alt-recap-subworker-1"

echo "======================================"
echo "Starting Classifier Retraining Pipeline (GPU)"
echo "======================================"

# 1. Sync Golden Data
echo "[1/7] Syncing Golden Data across project..."
if [ -f "$SOURCE_GOLDEN" ]; then
    cp "$SOURCE_GOLDEN" "./recap-worker/recap-worker/tests/data/golden_classification.json"
    cp "$SOURCE_GOLDEN" "./recap-worker/tests/data/golden_classification.json"
    echo "Files synced."
else
    echo "ERROR: Source golden dataset not found at $SOURCE_GOLDEN"
    exit 1
fi

# 2. Build Training Image
echo "[2/7] Building Training Image..."
docker build -t $TRAINING_IMAGE -f recap-subworker/Dockerfile.training recap-subworker

# 3. Prepare Dataset (GPU optional, but good for embedding)
echo "[3/7] Preparing Dataset..."
docker run --rm \
    --gpus all \
    -v "$HOST_DATA_DIR:/app/data" \
    $TRAINING_IMAGE \
    uv run python scripts/prepare_dataset.py \
    --input /app/data/golden_classification.json \
    --output_dir /app/data/dataset

# 4. Train Model (GPU)
echo "[4/7] Training Model..."
docker run --rm \
    --gpus all \
    -v "$HOST_DATA_DIR:/app/data" \
    $TRAINING_IMAGE \
    uv run python scripts/train_classifier_gpu.py \
    --data_dir /app/data/dataset \
    --output_model /app/data/genre_classifier.joblib \
    --output_thresholds /app/data/genre_thresholds.json

# 5. Copy Artifacts to Subworker
echo "[5/7] Deploying Artifacts to Service..."
# Since we mapped HOST_DATA_DIR to /app/data in training container,
# artifacts are already in recap-subworker/data on host.
# We need to copy them to the running subworker container IF it doesn't perform a live reload or volume mount.
# compose.yaml mounts ./recap-subworker/data/genre_classifier.joblib:/app/data/genre_classifier.joblib:ro
# So restarting the container should be enough to pick up changed host files.

# 6. Restart Service
echo "[6/7] Restarting Recap Subworker..."
docker restart $SUBWORKER_CONTAINER

echo "======================================"
echo "Retraining Complete!"
echo "Artifacts are in $HOST_DATA_DIR"
echo "Service $SUBWORKER_CONTAINER has been restarted."
echo "======================================"
