#!/bin/bash
# Generate dummy secrets for development

SECRETS_DIR="./secrets"
mkdir -p "$SECRETS_DIR"

# List of secrets to generate
SECRETS=(
    "auth_shared_secret.txt"
    "backend_token_secret.txt"
    "postgres_password.txt"
    "db_password.txt"
    "pre_processor_db_password.txt"
    "tag_generator_db_password.txt"
    "search_indexer_db_password.txt"
    "recap_db_password.txt"
    "kratos_db_password.txt"
    "kratos_cookie_secret.txt"
    "kratos_cipher_secret.txt"
    "meili_master_key.txt"
    "clickhouse_password.txt"
    "csrf_secret.txt"
    "service_secret.txt"
    "hugging_face_token.txt"
)

echo "Generating development secrets in $SECRETS_DIR..."

for secret in "${SECRETS[@]}"; do
    FILE="$SECRETS_DIR/$secret"
    if [ ! -f "$FILE" ]; then
        # Generate a random string or default value
        if [[ "$secret" == *"password"* || "$secret" == *"secret"* || "$secret" == *"key"* || "$secret" == *"token"* ]]; then
             openssl rand -hex 16 > "$FILE"
             # Remove trailing newline for cleaner usage in some contexts (though usually fine)
             truncate -s -1 "$FILE"
             echo "Generated $secret"
        else
             echo "dummy-value" > "$FILE"
             echo "Generated $secret"
        fi
    else
        echo "Skipping $secret (already exists)"
    fi
done

# Ensure hugging_face_token.txt is not empty if possible, or warn
if [ ! -s "$SECRETS_DIR/hugging_face_token.txt" ]; then
    echo "WARNING: hugging_face_token.txt is empty. Some ML features might fail."
fi

echo "Secrets generation complete."
