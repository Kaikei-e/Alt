#!/bin/bash
# Generate dummy secrets for development

SECRETS_DIR="./secrets"
mkdir -p "$SECRETS_DIR"

# List of secrets to generate
SECRETS=(
    "auth_shared_secret.txt"
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
    "internal_auth_token.txt"
    "irodori_api_key.txt"
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

# Backend token Ed25519 key pair: PKCS#8 private key, SPKI public key
SIGNING_KEY="$SECRETS_DIR/backend_token_signing_key.txt"
VERIFY_KEY="$SECRETS_DIR/backend_token_verify_key.txt"
if [ -f "$SIGNING_KEY" ] && [ -f "$VERIFY_KEY" ]; then
    echo "Skipping backend token key pair (already exists)"
elif [ -f "$SIGNING_KEY" ] || [ -f "$VERIFY_KEY" ]; then
    echo "ERROR: only one of backend_token_signing_key.txt and backend_token_verify_key.txt exists; remove it or restore the other" >&2
    exit 1
else
    if ! openssl genpkey -algorithm ED25519 -out "$SIGNING_KEY" ||
        ! openssl pkey -in "$SIGNING_KEY" -pubout -out "$VERIFY_KEY" ||
        ! chmod 644 "$SIGNING_KEY" "$VERIFY_KEY"; then
        rm -f "$SIGNING_KEY" "$VERIFY_KEY"
        echo "ERROR: could not generate the backend token Ed25519 key pair (is OpenSSL 1.1.1 or newer installed?)" >&2
        exit 1
    fi
    echo "Generated backend token key pair"
fi

# Ensure hugging_face_token.txt is not empty if possible, or warn
if [ ! -s "$SECRETS_DIR/hugging_face_token.txt" ]; then
    echo "WARNING: hugging_face_token.txt is empty. Some ML features might fail."
fi

echo "Secrets generation complete."
