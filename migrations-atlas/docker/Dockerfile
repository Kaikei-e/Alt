# Atlas Migration Container for Alt RSS Reader Database
# Based on 2025 Kubernetes-native migration best practices
FROM arigaio/atlas:latest-alpine

# Install PostgreSQL client for additional connectivity testing
RUN apk add --no-cache postgresql-client

# Create migration directory and set working directory
WORKDIR /migrations

# Copy all migration files from migrations directory (context is parent)
COPY migrations/ ./

# Create scripts directory for helper scripts
RUN mkdir -p /scripts

# Copy migration execution scripts from docker/scripts directory
COPY docker/scripts/ /scripts/

# Set permissions
RUN chmod +x /scripts/*.sh

# Fix ownership for non-root user
RUN chown -R 1001:1001 /migrations /scripts

# Set default user (non-root for security)
USER 1001:1001

# Override Atlas entrypoint to use our scripts
ENTRYPOINT ["/scripts/migrate.sh"]

# Default command runs migration status check
CMD ["status"]