# Atlas Migration Container for Recap DB
FROM arigaio/atlas:latest-alpine

RUN apk add --no-cache postgresql-client

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
