# Multi-stage build using official Playwright image as base
FROM mcr.microsoft.com/playwright:v1.54.0-noble as playwright-base

# Install required dependencies, Deno 2.0, and kubectl on Playwright base
RUN apt-get update && apt-get install -y unzip curl && \
    curl -fsSL https://deno.land/install.sh | sh && \
    mv /root/.deno/bin/deno /usr/local/bin/deno && \
    chmod +x /usr/local/bin/deno && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl && \
    rm -rf /var/lib/apt/lists/* && \
    apt-get clean

# Final stage
FROM playwright-base

LABEL maintainer="Alt RSS Reader Team"
LABEL description="Automated OAuth token refresh using Playwright + Deno 2.0"
LABEL version="2.0.0"

# Use existing pwuser from Playwright base image (UID 1000)
# and rename to authmanager for consistency
RUN usermod -l authmanager pwuser && \
    groupmod -n authmanager pwuser && \
    usermod -d /home/authmanager -m authmanager

# Set up working directory
WORKDIR /app

# Copy source code and configuration
COPY --chown=authmanager:authmanager deno.json main.ts ./
COPY --chown=authmanager:authmanager src/ ./src/

# Create required directories with proper permissions
RUN mkdir -p /app/.cache /app/logs /tmp/deno-cache \
    && chown -R authmanager:authmanager /app /home/authmanager /tmp/deno-cache \
    && chmod 755 /app/.cache /app/logs /tmp/deno-cache

# Cache Deno dependencies as root first
RUN DENO_DIR=/tmp/deno-cache deno cache main.ts \
    && chown -R authmanager:authmanager /tmp/deno-cache

# Install Playwright via Deno npm compatibility
USER authmanager
RUN cd /app && \
    deno run -A npm:playwright install chromium

# Set environment variables for optimal Deno + Playwright operation
ENV DENO_DIR=/tmp/deno-cache
ENV DENO_DEPLOYMENT_ID=auth-token-manager
ENV NO_COLOR=1
ENV DENO_NO_UPDATE_CHECK=1
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD deno run --allow-all main.ts health || exit 1

# Default command
CMD ["deno", "run", "--allow-all", "main.ts", "health"]

# Expose port for health checks (optional)
EXPOSE 8080

# Volume for temporary files
VOLUME ["/tmp"]

# Metadata
LABEL org.opencontainers.image.source="https://github.com/Kaikei-e/Alt"
LABEL org.opencontainers.image.description="Automated OAuth token refresh system for Alt RSS Reader"
LABEL org.opencontainers.image.licenses="MIT"