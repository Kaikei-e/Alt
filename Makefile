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

.PHONY: all up up-fresh up-clean build down down-volumes clean clean-env generate-mocks backup-db