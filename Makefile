# Makefile

# Docker context - use 'default' which has the most data and volumes
DOCKER_CONTEXT := default

# .env ファイルの名前
ENV_FILE := ./.env
# .env テンプレートファイルの名前
ENV_TEMPLATE := ./.env.template

# .env ファイルが存在する場合は読み込む
ifneq (,$(wildcard $(ENV_FILE)))
    include $(ENV_FILE)
    export
endif

# デフォルトのデータベース設定
POSTGRES_USER ?= devuser
POSTGRES_PASSWORD ?= devpassword
POSTGRES_DB ?= devdb

# デフォルトのターゲット
all: build up

# .env ファイルが存在しない場合に作成するターゲット
$(ENV_FILE): $(ENV_TEMPLATE)
	@echo "Checking for $(ENV_FILE)..."
	@if [ ! -f $(ENV_FILE) ]; then \
		echo "Creating $(ENV_FILE) from $(ENV_TEMPLATE)..."; \
		cp $(ENV_TEMPLATE) $(ENV_FILE); \
		echo "--------------------------------------------------------"; \
		echo "The $(ENV_FILE) file has been created from $(ENV_TEMPLATE)."; \
		echo "Please review and customize it with your specific environment variables, especially for sensitive data."; \
		echo "--------------------------------------------------------"; \
	else \
		echo "$(ENV_FILE) already exists. Skipping creation."; \
	fi

# docker-compose up --build -d を実行するターゲット
up: $(ENV_FILE)
	@echo "Setting Docker context to $(DOCKER_CONTEXT) (has most data/volumes)..."
	@docker context use $(DOCKER_CONTEXT) || true
	@echo "Starting Docker Compose services..."
	docker compose up --build -d

up-clean-frontend: $(ENV_FILE)
	@echo "Starting Docker Compose services with clean frontend build..."
	docker build --no-cache -f ./alt-frontend/Dockerfile.frontend -t alt-frontend ./alt-frontend
	docker compose up alt-frontend alt-backend db migrate nginx pre-processor --build -d

up-with-news-creator: $(ENV_FILE)
	@echo "Starting Docker Compose services with new creator build..."
	docker build --no-cache -f ./news-creator/Dockerfile.creator -t news-creator ./news-creator
	docker compose up --build -d

# Dockerイメージをビルドするターゲット (個別実行も可能)
build: $(ENV_FILE)
	@echo "Setting Docker context to $(DOCKER_CONTEXT)..."
	@docker context use $(DOCKER_CONTEXT) || true
	@echo "Building Docker images..."
	docker compose build

# サービスを停止するターゲット
down:
	@echo "Setting Docker context to $(DOCKER_CONTEXT)..."
	@docker context use $(DOCKER_CONTEXT) || true
	@echo "Stopping Docker Compose services..."
	docker compose down

# ボリュームも削除してサービスを停止するターゲット (開発時によく使う)
down-volumes:
	@echo "Stopping Docker Compose services and removing volumes..."
	docker compose down --volumes

# コンテナ、イメージ、ボリュームなどを全てクリーンアップするターゲット
clean: down-volumes
	@echo "Cleaning up dangling images and build cache..."
	docker system prune -f --all # 全ての未使用のコンテナ、ネットワーク、イメージを削除
	docker builder prune -f      # ビルドキャッシュを削除

# .env ファイルを削除するターゲット
clean-env:
	@echo "Removing $(ENV_FILE)..."
	@rm -f $(ENV_FILE)

PORT_BASE_DIR := ./alt-backend/app/port
MOCKS_DIR := ./alt-backend/app/mocks

TAG_GENERATOR_DIR := ./tag-generator
TAG_ONNX_DIR := $(TAG_GENERATOR_DIR)/models/onnx
TAG_ONNX_VENV := $(TAG_GENERATOR_DIR)/.onnx-venv
TAG_ONNX_MODEL := $(TAG_ONNX_DIR)/model.onnx

# Buf Connect-RPC code generation
buf-generate:
	@echo "Generating Connect-RPC code from proto files..."
	@cd proto && buf generate
	@echo "Connect-RPC code generated successfully."

buf-lint:
	@echo "Linting proto files..."
	@cd proto && buf lint

buf-breaking:
	@echo "Checking for breaking changes..."
	@cd proto && buf breaking --against '.git#branch=main'

generate-mocks:
	@echo "Generating GoMock mocks for all interfaces in $(PORT_BASE_DIR)..."
	@mkdir -p $(MOCKS_DIR)
	@find $(PORT_BASE_DIR) -name "*.go" | while read -r file; do \
		package_name=$$(basename $$(dirname $$file)); \
		interface_name=$$(grep -Eo "type [[:alpha:]]+ interface" $$file | awk '{print $$2}' | head -n 1); \
		if [ -n "$$interface_name" ]; then \
			echo "  - Generating mock for interface '$$interface_name' from '$$file'"; \
			mockgen -source=$$file \
				-destination=$(MOCKS_DIR)/mock_$$package_name.go \
				-package=mocks \
				$$interface_name; \
		else \
			echo "  - No interface found in '$$file', skipping."; \
		fi; \
	done
	@echo "GoMock mocks generated successfully in $(MOCKS_DIR)/"

backup-db:
	@echo "Backing up database..."
	docker compose exec db pg_dump -U $(POSTGRES_USER) -h localhost -p 5432 $(POSTGRES_DB) > backup.sql

dev-ssl-setup:
	@echo "Generating development SSL certificates..."
	@chmod +x docker/postgres/generate-dev-certs.sh
	@./docker/postgres/generate-dev-certs.sh
	@echo "SSL certificates generated. You can now start the services with SSL."

dev-ssl-test:
	@echo "Testing SSL connection..."
	@docker compose exec db psql \
		-h localhost -U ${POSTGRES_USER:-devuser} -d ${POSTGRES_DB:-devdb} \
		-c "SELECT ssl_is_used(), version();"

dev-clean-ssl:
	@echo "Cleaning SSL certificates..."
	@rm -rf docker/postgres/ssl/
	@echo "SSL certificates removed."

# Migration management targets
migrate-hash:
	@echo "Regenerating atlas.sum checksum file..."
	@docker run --rm \
		-v $(PWD)/migrations-atlas/migrations:/migrations:rw \
		--user 0:0 \
		--entrypoint /scripts/hash.sh \
		alt-migrate
	@echo "atlas.sum regenerated successfully. You can now run 'make up' or 'docker compose up migrate'."

migrate-validate:
	@echo "Validating migration files (offline check)..."
	@docker compose run --rm --no-deps migrate syntax-check

migrate-status:
	@echo "Checking migration status..."
	@docker compose run --rm migrate status

backfill-feed-ids:
	@echo "Backfilling article feed_ids from matching feed links..."
	@echo "This is a data migration script (not a schema migration)."
	@docker compose run --rm tag-generator python3 /scripts/backfill_article_feed_ids.py

recap-migrate-hash:
	@echo "Regenerating recap-worker atlas.sum checksum file..."
	@docker compose --profile recap build recap-db-migrator
	@docker run --rm \
		-v $(PWD)/recap-migration-atlas/migrations:/migrations:rw \
		--user 0:0 \
		--entrypoint /scripts/hash.sh \
		alt-recap-db-migrator
	@echo "atlas.sum regenerated successfully. You can now run 'make recap-migrate'."

recap-migrate:
	@echo "Applying recap-worker database migrations..."
	@docker compose --profile recap run --rm recap-db-migrator

recap-migrate-status:
	@echo "Checking recap-worker database migration status..."
	@docker compose --profile recap run --rm recap-db-migrator status

# Docker disk space management targets
docker-cleanup:
	@echo "Running Docker disk space cleanup..."
	@./scripts/docker-cleanup.sh

docker-cleanup-install:
	@echo "Installing Docker cleanup systemd timer..."
	@sudo cp scripts/docker-cleanup.service /etc/systemd/system/
	@sudo cp scripts/docker-cleanup.timer /etc/systemd/system/
	@sudo systemctl daemon-reload
	@sudo systemctl enable docker-cleanup.timer
	@sudo systemctl start docker-cleanup.timer
	@echo "Docker cleanup timer installed and started."
	@echo "To check status: sudo systemctl status docker-cleanup.timer"
	@echo "To view logs: sudo journalctl -u docker-cleanup.service"

docker-cleanup-uninstall:
	@echo "Uninstalling Docker cleanup systemd timer..."
	@sudo systemctl stop docker-cleanup.timer || true
	@sudo systemctl disable docker-cleanup.timer || true
	@sudo rm -f /etc/systemd/system/docker-cleanup.service
	@sudo rm -f /etc/systemd/system/docker-cleanup.timer
	@sudo systemctl daemon-reload
	@echo "Docker cleanup timer uninstalled."

docker-cleanup-status:
	@echo "Docker cleanup timer status:"
	@sudo systemctl status docker-cleanup.timer --no-pager || true
	@echo ""
	@echo "Last cleanup run:"
	@sudo journalctl -u docker-cleanup.service -n 20 --no-pager || true

docker-disk-usage:
	@echo "Current Docker disk usage:"
	@docker system df
	@echo ""
	@echo "Detailed breakdown:"
	@docker system df -v

# Memory-focused cleanup targets
docker-cleanup-memory:
	@echo "=== Docker Memory Cleanup ==="
	@echo "This will free up memory by removing unused Docker resources."
	@echo ""
	@echo "1. Removing stopped containers..."
	@docker container prune -f || true
	@echo ""
	@echo "2. Removing unused images (older than 24h)..."
	@docker image prune -a -f --filter "until=24h" || true
	@echo ""
	@echo "3. Removing build cache..."
	@docker builder prune -f --filter "until=24h" || true
	@echo ""
	@echo "4. Removing unused volumes (excluding active ones)..."
	@docker volume prune -f || true
	@echo ""
	@echo "5. Removing unused networks..."
	@docker network prune -f || true
	@echo ""
	@echo "6. Cleaning up old logs..."
	@docker compose logs --tail=0 2>/dev/null || true
	@echo ""
	@echo "=== Cleanup Complete ==="
	@echo "Current Docker resource usage:"
	@docker system df

docker-cleanup-memory-aggressive:
	@echo "=== Aggressive Docker Memory Cleanup ==="
	@echo "WARNING: This will remove ALL unused resources, including recent ones."
	@echo ""
	@read -p "Are you sure? (yes/no): " confirm && [ "$$confirm" = "yes" ] || exit 1
	@echo ""
	@echo "1. Removing all stopped containers..."
	@docker container prune -f || true
	@echo ""
	@echo "2. Removing all unused images..."
	@docker image prune -a -f || true
	@echo ""
	@echo "3. Removing all build cache..."
	@docker builder prune -a -f || true
	@echo ""
	@echo "4. Removing unused volumes (CAREFUL: may remove data)..."
	@docker volume prune -f || true
	@echo ""
	@echo "5. Removing unused networks..."
	@docker network prune -f || true
	@echo ""
	@echo "6. System-wide cleanup..."
	@docker system prune -a -f --volumes || true
	@echo ""
	@echo "=== Aggressive Cleanup Complete ==="
	@echo "Current Docker resource usage:"
	@docker system df

docker-remove-old-volumes:
	@echo "=== Removing Old/Unused Volumes ==="
	@echo "WARNING: This will remove unused volumes. Active volumes will be preserved."
	@echo ""
	@echo "Checking for old db_data volume (PostgreSQL 16, no longer used)..."
	@if docker volume inspect alt_db_data >/dev/null 2>&1; then \
		echo "Found old db_data volume. Removing..."; \
		docker volume rm alt_db_data 2>/dev/null || echo "Could not remove alt_db_data (may be in use)"; \
	else \
		echo "No old db_data volume found."; \
	fi
	@if docker volume inspect alt-db_data >/dev/null 2>&1; then \
		echo "Found old db_data volume (alt-db_data). Removing..."; \
		docker volume rm alt-db_data 2>/dev/null || echo "Could not remove alt-db_data (may be in use)"; \
	fi
	@echo ""
	@echo "Removing all unused volumes (safe - only removes volumes not attached to any container)..."
	@docker volume prune -f || true
	@echo "=== Volume Cleanup Complete ==="

docker-memory-stats:
	@echo "=== Docker Memory Usage Statistics ==="
	@echo ""
	@echo "Container memory usage:"
	@docker stats --no-stream --format "table {{.Container}}\t{{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}" 2>/dev/null || echo "No running containers"
	@echo ""
	@echo "Docker system disk usage:"
	@docker system df
	@echo ""
	@echo "Top memory-consuming containers:"
	@docker stats --no-stream --format "table {{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}" --sort mem 2>/dev/null | head -10 || echo "No running containers"

prepare-tag-onnx: $(TAG_ONNX_MODEL)
	@echo "tag-generator ONNX model is available at $(TAG_ONNX_MODEL)"

$(TAG_ONNX_MODEL):
	@echo "Preparing SentenceTransformer ONNX model for tag-generator..."
	@mkdir -p $(TAG_ONNX_DIR)
	@if [ ! -d $(TAG_ONNX_VENV) ]; then \
		echo "Creating dedicated virtual environment at $(TAG_ONNX_VENV)..."; \
		python3 -m venv $(TAG_ONNX_VENV); \
	fi
	@echo "Installing conversion dependencies (optimum[onnxruntime])..."
	@$(TAG_ONNX_VENV)/bin/pip install --upgrade pip >/dev/null
	@$(TAG_ONNX_VENV)/bin/pip install --quiet "optimum[onnxruntime]>=1.21.0"
	@echo "Converting SentenceTransformer to ONNX..."
	@ONNX_OUTPUT_DIR=$(TAG_ONNX_DIR) HF_TOKEN=$(HF_TOKEN) $(TAG_ONNX_VENV)/bin/python $(TAG_GENERATOR_DIR)/scripts/convert_to_onnx.py
	@echo "✅ ONNX model stored at $(TAG_ONNX_MODEL)"

clean-tag-onnx:
	@echo "Removing generated ONNX artifacts and virtual environment..."
	@rm -rf $(TAG_ONNX_DIR) $(TAG_ONNX_VENV)
	@echo "tag-generator ONNX assets cleaned."

.PHONY: all up up-fresh up-clean build down down-volumes clean clean-env generate-mocks backup-db dev-ssl-setup dev-ssl-test dev-clean-ssl migrate-hash migrate-validate migrate-status recap-migrate-hash recap-migrate recap-migrate-status docker-cleanup docker-cleanup-install docker-cleanup-uninstall docker-cleanup-status docker-disk-usage docker-cleanup-memory docker-cleanup-memory-aggressive docker-remove-old-volumes docker-memory-stats prepare-tag-onnx clean-tag-onnx buf-generate buf-lint buf-breaking