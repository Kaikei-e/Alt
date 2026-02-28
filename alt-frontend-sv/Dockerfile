# syntax=docker/dockerfile:1
# ==============================================================================
# alt-frontend-sv Dockerfile
# SvelteKit + Bun multi-stage build (optimized)
# ==============================================================================

FROM oven/bun:1.3.10-alpine AS base
WORKDIR /app

# ---------- All deps (for building) ----------
FROM base AS deps
COPY package.json bun.lock ./
RUN --mount=type=cache,id=bun-alt-frontend-sv,target=/root/.bun/install/cache \
    bun install --frozen-lockfile

# ---------- Production deps only (for runtime) ----------
FROM base AS prod-deps
COPY package.json bun.lock ./
RUN --mount=type=cache,id=bun-alt-frontend-sv,target=/root/.bun/install/cache \
    bun install --frozen-lockfile --production

# ---------- Build ----------
FROM base AS builder
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NODE_ENV=production
RUN bun run prepare && bun run build

# ---------- Runtime ----------
FROM oven/bun:1.3.10-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production \
    PORT=4173 \
    HOST=0.0.0.0

RUN addgroup -g 1001 -S sveltekit && \
    adduser -u 1001 -S -G sveltekit sveltekit

COPY --from=builder --chown=sveltekit:sveltekit /app/build ./build
COPY --from=prod-deps --chown=sveltekit:sveltekit /app/node_modules ./node_modules
COPY --chown=sveltekit:sveltekit package.json ./

USER sveltekit
EXPOSE 4173
CMD ["bun", "build/index.js"]
