# Makefile

# .env ファイルの名前
ENV_FILE := ./.env
# .env テンプレートファイルの名前
ENV_TEMPLATE := ./.env.template

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

.PHONY: all up build down down-volumes clean clean-env