# alt-perf Dockerfile
# Deno-based E2E performance measurement tool

FROM denoland/deno:debian-2.4.0

WORKDIR /app

# Install Google Chrome and minimal dependencies
RUN apt-get update \
    && apt-get install -y wget gnupg ca-certificates \
    && wget -q -O - https://dl-ssl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /usr/share/keyrings/googlechrome-linux-keyring.gpg \
    && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/googlechrome-linux-keyring.gpg] https://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google.list \
    && apt-get update \
    && apt-get install -y google-chrome-stable fonts-liberation fonts-noto-cjk libxss1 --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

# Set environment for Chrome
ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
ENV PUPPETEER_EXECUTABLE_PATH=/usr/bin/google-chrome-stable
ENV CHROME_BIN=/usr/bin/google-chrome-stable
ENV HOME=/home/deno

# Create home directory for deno user (needed by Chrome)
RUN mkdir -p /home/deno/.local/share/applications \
    && mkdir -p /home/deno/.cache \
    && chown -R deno:deno /home/deno

# Copy project files
COPY --chown=deno:deno deno.json deno.lock* ./
COPY --chown=deno:deno config/ ./config/
COPY --chown=deno:deno src/ ./src/
COPY --chown=deno:deno main.ts ./

# Create reports directory
RUN mkdir -p /app/reports && chown -R deno:deno /app/reports

# Cache dependencies
RUN deno cache main.ts

# Switch to non-root user
USER deno

# Set default entrypoint
ENTRYPOINT ["deno", "run", "--allow-all", "main.ts"]

# Default command shows help
CMD ["help"]
