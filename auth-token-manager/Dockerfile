FROM denoland/deno:2.6.3

# Set working directory
WORKDIR /app

# Cache dependencies
# Copy only the main file first to cache dependencies if possible,
# or if using deno.json/import_map, copy those first.
# Since we only have main.ts and src, we copy everything but ideally we'd cache first.
# For now, simplistic approach is fine as Deno caches based on import URLs.

COPY . .

# Cache dependencies by compiling/caching main entry point
RUN deno cache main.ts

# Set environment variables
ENV TOKEN_STORAGE_PATH=/app/secrets/oauth2_token.env

# Default command to run the daemon
CMD ["run", "--allow-all", "main.ts", "daemon"]