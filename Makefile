# Makefile

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
	@echo "Building Docker images..."
	docker compose build

# サービスを停止するターゲット
down:
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

recap-migrate:
	@echo "Applying recap-worker database migrations..."
	@docker compose run --rm recap-worker sqlx migrate run

recap-migrate-status:
	@echo "Checking recap-worker database migration status..."
	@docker compose run --rm recap-worker sqlx migrate info

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

.PHONY: all up up-fresh up-clean build down down-volumes clean clean-env generate-mocks backup-db dev-ssl-setup dev-ssl-test dev-clean-ssl migrate-hash migrate-validate migrate-status recap-migrate recap-migrate-status docker-cleanup docker-cleanup-install docker-cleanup-uninstall docker-cleanup-status docker-disk-usage