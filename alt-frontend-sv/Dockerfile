# ==============================================================================
# alt-frontend-sv Dockerfile
# SvelteKit + Bun マルチステージビルド
# ==============================================================================

FROM oven/bun:1-alpine AS base
WORKDIR /app

# 依存関係ステージ
FROM base AS deps
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

# ビルドステージ
FROM base AS builder
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN bun run prepare
RUN bun run build

# ランタイムステージ
FROM oven/bun:1-alpine AS runner
WORKDIR /app

# 環境変数
ENV NODE_ENV=production \
    PORT=4173 \
    HOST=0.0.0.0

# 非rootユーザー作成
RUN addgroup -g 1001 -S sveltekit && \
    adduser -u 1001 -S -G sveltekit sveltekit

# ビルド成果物をコピー
COPY --from=builder --chown=sveltekit:sveltekit /app/build ./build
COPY --from=builder --chown=sveltekit:sveltekit /app/node_modules ./node_modules
COPY --chown=sveltekit:sveltekit package.json ./

USER sveltekit

EXPOSE 4173

CMD ["bun", "build/index.js"]
