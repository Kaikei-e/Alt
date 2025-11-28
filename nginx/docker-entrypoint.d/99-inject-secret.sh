#!/bin/sh
set -e

# Define the secret file path (mounted via Docker Secrets)
SECRET_FILE="/run/secrets/auth_shared_secret"
NGINX_CONF="/etc/nginx/conf.d/default.conf"

# Check if the secret file exists
if [ -f "$SECRET_FILE" ]; then
    echo "Injecting auth_shared_secret into Nginx configuration..."

    # Read the secret from the file
    SECRET=$(cat "$SECRET_FILE")

    # Use sed to replace the placeholder in the configuration file
    # We use a different delimiter (|) to avoid issues if the secret contains slashes
    sed -i "s|__AUTH_SHARED_SECRET__|$SECRET|g" "$NGINX_CONF"

    echo "Secret injection complete."
else
    echo "Warning: Secret file $SECRET_FILE not found. Nginx will start with placeholder secret."
fi
