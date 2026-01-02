# ==============================================================================
# alt-perf Dockerfile
# Deno-based E2E パフォーマンス測定ツール
# Chrome が必要なため distroless 使用不可
# ==============================================================================

FROM denoland/deno:debian-2.4.0

WORKDIR /app

# Chrome と必要最小限の依存関係をインストール
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    wget gnupg ca-certificates \
    && wget -q -O - https://dl-ssl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /usr/share/keyrings/googlechrome-linux-keyring.gpg \
    && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/googlechrome-linux-keyring.gpg] https://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
    google-chrome-stable fonts-liberation fonts-noto-cjk libxss1 \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Chrome 環境変数
ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true \
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/google-chrome-stable \
    CHROME_BIN=/usr/bin/google-chrome-stable \
    HOME=/home/deno

# deno ユーザー用ディレクトリ作成 (Chrome 必須)
RUN mkdir -p /home/deno/.local/share/applications \
    && mkdir -p /home/deno/.cache \
    && chown -R deno:deno /home/deno

# 依存関係ファイルを先にコピー (キャッシュ効率化)
COPY --chown=deno:deno deno.json deno.lock* ./

# 依存関係をキャッシュ
RUN deno cache --lock=deno.lock main.ts 2>/dev/null || true

# プロジェクトファイルをコピー
COPY --chown=deno:deno config/ ./config/
COPY --chown=deno:deno src/ ./src/
COPY --chown=deno:deno main.ts ./

# レポートディレクトリ作成
RUN mkdir -p /app/reports && chown -R deno:deno /app/reports

# 依存関係を再キャッシュ (全ファイル込み)
RUN deno cache main.ts

# 非rootユーザーに切り替え
USER deno

# E2Eテストには広範な権限が必要
ENTRYPOINT ["deno", "run", "--allow-all", "main.ts"]

CMD ["help"]
