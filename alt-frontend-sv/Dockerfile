# syntax=docker/dockerfile:1.7
# ==============================================================================
# alt-frontend-sv Dockerfile
# SvelteKit + Bun multi-stage build
# ==============================================================================

FROM oven/bun:1.3.12-alpine AS base
WORKDIR /app

# ---------- Install (dev + prod in one stage, per Bun official guide) ----------
FROM base AS install
RUN --mount=type=cache,id=bun-alt-frontend-sv,target=/root/.bun/install/cache \
    mkdir -p /temp/dev /temp/prod

COPY --link package.json bun.lock /temp/dev/
RUN --mount=type=cache,id=bun-alt-frontend-sv,target=/root/.bun/install/cache \
    cd /temp/dev && bun install --frozen-lockfile

COPY --link package.json bun.lock /temp/prod/
RUN --mount=type=cache,id=bun-alt-frontend-sv,target=/root/.bun/install/cache \
    cd /temp/prod && bun install --frozen-lockfile --production

# ---------- Build ----------
FROM base AS builder
COPY --link --from=install /temp/dev/node_modules ./node_modules
COPY --link . .
ENV NODE_ENV=production
RUN --mount=type=cache,id=vite-alt-frontend-sv,target=/app/node_modules/.vite \
    --mount=type=cache,id=sveltekit-alt-frontend-sv,target=/app/.svelte-kit \
    bun run prepare && bun run build

# ---------- Runtime ----------
FROM oven/bun:1.3.12-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production \
    PORT=4173 \
    HOST=0.0.0.0

RUN addgroup -g 1001 -S sveltekit && \
    adduser -u 1001 -S -G sveltekit sveltekit

COPY --link --from=builder --chown=1001:1001 /app/build ./build
COPY --link --from=install --chown=1001:1001 /temp/prod/node_modules ./node_modules
COPY --link --chown=1001:1001 package.json ./

USER sveltekit
EXPOSE 4173
CMD ["bun", "build/index.js"]
