# Atlas Migration Container for Pre-Processor DB
FROM arigaio/atlas:0.31.0-alpine

# Connectivity check uses busybox `nc` (already in arigaio/atlas:*-alpine).
# The previous apk install was the sole network dependency at build time
# and could hang for hours on slow dl-cdn.alpinelinux.org responses
# (observed 2026-04-30, alt-deploy run 25143212120).

WORKDIR /migrations

COPY migrations/ ./

RUN mkdir -p /scripts
COPY docker/scripts/migrate.sh /scripts/
COPY docker/scripts/hash.sh /scripts/

RUN chmod +x /scripts/*.sh
RUN chown -R 1001:1001 /migrations /scripts

USER 1001:1001

ENTRYPOINT ["/scripts/migrate.sh"]
CMD ["status"]
