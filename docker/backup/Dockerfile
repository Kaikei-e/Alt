# docker/backup/Dockerfile - restic-backup image with pinned, checksum-verified supercronic.
#
# Previously restic-backup downloaded supercronic from GitHub on every
# container start with no checksum verification (A08:2025 Software & Data
# Integrity Failures / CWE-494). This bakes the download into the image
# build instead, verified with sha256sum -c (same pattern as
# recap-worker/Dockerfile.recap-worker's libtorch fetch).
FROM restic/restic:0.17.3

ARG SUPERCRONIC_VERSION=v0.2.33
ARG SUPERCRONIC_AMD64_SHA256=feefa310da569c81b99e1027b86b27b51e6ee9ab647747b49099645120cfc671
ARG SUPERCRONIC_ARM64_SHA256=f1f8585c66de020fef494dd636058f99949d108f569fef00016a1c8b9eb145b3

RUN apk add --no-cache curl bash jq docker-cli && \
  ARCH="$(uname -m)" && \
  if [ "$ARCH" = "aarch64" ]; then \
  SC_ARCH=arm64; SC_SHA256="$SUPERCRONIC_ARM64_SHA256"; \
  else \
  SC_ARCH=amd64; SC_SHA256="$SUPERCRONIC_AMD64_SHA256"; \
  fi && \
  curl -fsSL -o /usr/local/bin/supercronic \
  "https://github.com/aptible/supercronic/releases/download/${SUPERCRONIC_VERSION}/supercronic-linux-${SC_ARCH}" && \
  echo "${SC_SHA256}  /usr/local/bin/supercronic" | sha256sum -c - && \
  chmod +x /usr/local/bin/supercronic

ENTRYPOINT ["/usr/local/bin/supercronic"]
CMD ["/scripts/crontab"]
