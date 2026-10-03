#!/bin/sh
set -e

# Read secrets from files (remove trailing newline and reject empty/newline-only content)
read_secret() {
    file=$1
    if [ ! -f "$file" ]; then
        echo "Error: secret file $(basename "$file") is missing" >&2
        exit 1
    fi
    val=$(tr -d '\r\n' < "$file")
    if [ -z "$val" ]; then
        echo "Error: secret file $(basename "$file") is empty" >&2
        exit 1
    fi
    printf '%s' "$val"
}

# UTF-8 RFC 3986 URI component encoding (unreserved: 0-9, A-Z, a-z, -, _, ., ~)
uri_encode() {
    printf '%s' "$1" | od -A n -v -t u1 | awk '
    {
        for (i = 1; i <= NF; i++) {
            b = $i + 0
            if ((b >= 48 && b <= 57) || (b >= 65 && b <= 90) || (b >= 97 && b <= 122) || b == 45 || b == 95 || b == 46 || b == 126) {
                printf "%c", b
            } else {
                printf "%%%02X", b
            }
        }
    }
    '
}

# Formats string as a JSON double-quoted scalar, valid as a YAML string scalar
to_json_scalar() {
    printf '%s' "$1" | od -A n -v -t u1 | awk '
    BEGIN {
        printf "\""
    }
    {
        for (i = 1; i <= NF; i++) {
            b = $i + 0
            if (b == 92) {
                printf "\\\\"
            } else if (b == 34) {
                printf "\\\""
            } else if (b == 10) {
                printf "\\n"
            } else if (b == 13) {
                printf "\\r"
            } else if (b == 9) {
                printf "\\t"
            } else if (b < 32 || b == 127) {
                printf "\\u%04x", b
            } else {
                printf "%c", b
            }
        }
    }
    END {
        printf "\""
    }
    '
}

escape_sed() {
    printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}

KRATOS_DB_PASSWORD_FILE=${KRATOS_DB_PASSWORD_FILE:-/run/secrets/kratos_db_password}
KRATOS_COOKIE_SECRET_FILE=${KRATOS_COOKIE_SECRET_FILE:-/run/secrets/kratos_cookie_secret}
KRATOS_CIPHER_SECRET_FILE=${KRATOS_CIPHER_SECRET_FILE:-/run/secrets/kratos_cipher_secret}

KRATOS_DB_PASSWORD=$(read_secret "$KRATOS_DB_PASSWORD_FILE")
export KRATOS_DB_PASSWORD
KRATOS_COOKIE_SECRET=$(read_secret "$KRATOS_COOKIE_SECRET_FILE")
export KRATOS_COOKIE_SECRET
KRATOS_CIPHER_SECRET=$(read_secret "$KRATOS_CIPHER_SECRET_FILE")
export KRATOS_CIPHER_SECRET

# Construct DSN with RFC 3986 percent-encoded credentials
DB_USER=${KRATOS_DB_USER:-kratos_user}
DB_HOST=${KRATOS_DB_HOST:-kratos-db}
DB_PORT=${KRATOS_DB_PORT:-5432}
DB_NAME=${KRATOS_DB_NAME:-kratos}
DB_SSLMODE=${KRATOS_DB_SSLMODE:-disable}

DSN_PARAMS="sslmode=${DB_SSLMODE}&max_conns=30&max_idle_conns=15"
if [ "$DB_PORT" = "6432" ]; then
    DSN_PARAMS="${DSN_PARAMS}&prepared_statements=false"
fi

ENC_USER=$(uri_encode "$DB_USER")
ENC_PASS=$(uri_encode "$KRATOS_DB_PASSWORD")
export DSN="postgres://${ENC_USER}:${ENC_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?${DSN_PARAMS}"

# Public and UI URLs parameterization with production defaults
KRATOS_PUBLIC_URL=${KRATOS_PUBLIC_URL:-https://example.com/ory}
KRATOS_DEFAULT_BROWSER_RETURN_URL=${KRATOS_DEFAULT_BROWSER_RETURN_URL:-https://example.com/}
KRATOS_LOGIN_UI_URL=${KRATOS_LOGIN_UI_URL:-https://example.com/auth/login}
KRATOS_REGISTRATION_UI_URL=${KRATOS_REGISTRATION_UI_URL:-https://example.com/auth/register}
KRATOS_ERROR_UI_URL=${KRATOS_ERROR_UI_URL:-https://example.com/auth/error}
KRATOS_SETTINGS_UI_URL=${KRATOS_SETTINGS_UI_URL:-https://example.com/auth/settings}
KRATOS_RECOVERY_UI_URL=${KRATOS_RECOVERY_UI_URL:-https://example.com/auth/recovery}
KRATOS_VERIFICATION_UI_URL=${KRATOS_VERIFICATION_UI_URL:-https://example.com/auth/verification}

TEMPLATE_FILE=${KRATOS_TEMPLATE_FILE:-/etc/config/kratos/kratos_template.yml}
if [ ! -f "$TEMPLATE_FILE" ]; then
    echo "Error: canonical template $TEMPLATE_FILE not found." >&2
    exit 1
fi

# Empty is a deliberate host-only cookie; unset would silently render one too.
if grep -qF "\${KRATOS_COOKIE_DOMAIN}" "$TEMPLATE_FILE"; then
    : "${KRATOS_COOKIE_DOMAIN?must be set for this template (empty means host-only cookies)}"
fi

KRATOS_CONFIG_FILE=${KRATOS_CONFIG_FILE:-/tmp/kratos.yml}
export KRATOS_CONFIG_FILE

# Escape JSON string scalars for safe sed delimiter replacement without leaking secrets
ESC_COOKIE=$(escape_sed "$(to_json_scalar "$KRATOS_COOKIE_SECRET")")
ESC_CIPHER=$(escape_sed "$(to_json_scalar "$KRATOS_CIPHER_SECRET")")
ESC_DSN=$(escape_sed "$(to_json_scalar "$DSN")")

ESC_PUBLIC_URL=$(escape_sed "$(to_json_scalar "$KRATOS_PUBLIC_URL")")
ESC_RETURN_URL=$(escape_sed "$(to_json_scalar "$KRATOS_DEFAULT_BROWSER_RETURN_URL")")
ESC_LOGIN_URL=$(escape_sed "$(to_json_scalar "$KRATOS_LOGIN_UI_URL")")
ESC_REG_URL=$(escape_sed "$(to_json_scalar "$KRATOS_REGISTRATION_UI_URL")")
ESC_ERR_URL=$(escape_sed "$(to_json_scalar "$KRATOS_ERROR_UI_URL")")
ESC_SETT_URL=$(escape_sed "$(to_json_scalar "$KRATOS_SETTINGS_UI_URL")")
ESC_REC_URL=$(escape_sed "$(to_json_scalar "$KRATOS_RECOVERY_UI_URL")")
ESC_VER_URL=$(escape_sed "$(to_json_scalar "$KRATOS_VERIFICATION_UI_URL")")
ESC_COOKIE_DOMAIN=$(escape_sed "$(to_json_scalar "$KRATOS_COOKIE_DOMAIN")")

CONFIG_DIR=$(dirname "$KRATOS_CONFIG_FILE")
TMP_CONFIG=$(mktemp "${CONFIG_DIR}/.kratos.yml.tmp.XXXXXX")
chmod 0600 "$TMP_CONFIG"

sed -e "s|\${KRATOS_COOKIE_SECRET}|${ESC_COOKIE}|g" \
    -e "s|\${KRATOS_CIPHER_SECRET}|${ESC_CIPHER}|g" \
    -e "s|\${DSN}|${ESC_DSN}|g" \
    -e "s|\${KRATOS_PUBLIC_URL}|${ESC_PUBLIC_URL}|g" \
    -e "s|\${KRATOS_DEFAULT_BROWSER_RETURN_URL}|${ESC_RETURN_URL}|g" \
    -e "s|\${KRATOS_LOGIN_UI_URL}|${ESC_LOGIN_URL}|g" \
    -e "s|\${KRATOS_REGISTRATION_UI_URL}|${ESC_REG_URL}|g" \
    -e "s|\${KRATOS_ERROR_UI_URL}|${ESC_ERR_URL}|g" \
    -e "s|\${KRATOS_SETTINGS_UI_URL}|${ESC_SETT_URL}|g" \
    -e "s|\${KRATOS_RECOVERY_UI_URL}|${ESC_REC_URL}|g" \
    -e "s|\${KRATOS_VERIFICATION_UI_URL}|${ESC_VER_URL}|g" \
    -e "s|\${KRATOS_COOKIE_DOMAIN}|${ESC_COOKIE_DOMAIN}|g" \
    "$TEMPLATE_FILE" > "$TMP_CONFIG"

chmod 0600 "$TMP_CONFIG"
mv -f "$TMP_CONFIG" "$KRATOS_CONFIG_FILE"
chmod 0600 "$KRATOS_CONFIG_FILE"

# Parse argv to replace --config value, preserve argv directly
set -- "$@"
replace_next=0
for arg do
    if [ "$replace_next" -eq 1 ]; then
        set -- "$@" "$KRATOS_CONFIG_FILE"
        replace_next=0
    elif [ "$arg" = "--config" ]; then
        set -- "$@" "--config"
        replace_next=1
    else
        case "$arg" in
            --config=)
                echo "Error: missing argument for --config" >&2
                exit 1
                ;;
            --config=*)
                set -- "$@" "--config=$KRATOS_CONFIG_FILE"
                ;;
            *)
                set -- "$@" "$arg"
                ;;
        esac
    fi
    shift
done

if [ "$replace_next" -eq 1 ]; then
    echo "Error: missing argument for --config" >&2
    exit 1
fi

exec "$@"
