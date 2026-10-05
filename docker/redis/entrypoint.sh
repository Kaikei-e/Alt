#!/bin/sh
set -e

# Read and validate secret, returning its SHA256 hex digest
# Strips trailing \r\n, rejects missing, empty, control, malformed UTF-8, and whitespace/injection
# Never echoes secret content or quotes invalid lines in error messages
get_secret_hash() {
    file=$1
    label=$2
    if [ ! -f "$file" ]; then
        echo "Error: secret file for $label is missing ($file)" >&2
        exit 1
    fi

    status=0
    clean_bytes=$(od -A n -v -t u1 "$file" | LC_ALL=C awk '
    {
        for (i = 1; i <= NF; i++) {
            n++
            b[n] = $i + 0
        }
    }
    END {
        if (n > 0 && b[n] == 10) {
            n--
            if (n > 0 && b[n] == 13) {
                n--
            }
        } else if (n > 0 && b[n] == 13) {
            n--
        }
        if (n == 0) {
            exit 1
        }
        for (i = 1; i <= n; i++) {
            val = b[i]
            if (val <= 32 || val == 127) {
                exit 2
            } else if (val >= 128) {
                if (val >= 194 && val <= 223) {
                    min2 = 128; max2 = 191; len = 2
                } else if (val == 224) {
                    min2 = 160; max2 = 191; len = 3
                } else if (val >= 225 && val <= 236) {
                    min2 = 128; max2 = 191; len = 3
                } else if (val == 237) {
                    min2 = 128; max2 = 159; len = 3
                } else if (val >= 238 && val <= 239) {
                    min2 = 128; max2 = 191; len = 3
                } else if (val == 240) {
                    min2 = 144; max2 = 191; len = 4
                } else if (val >= 241 && val <= 243) {
                    min2 = 128; max2 = 191; len = 4
                } else if (val == 244) {
                    min2 = 128; max2 = 143; len = 4
                } else {
                    exit 2
                }
                i++
                if (i > n || b[i] < min2 || b[i] > max2) exit 2
                for (j = 3; j <= len; j++) {
                    i++
                    if (i > n || b[i] < 128 || b[i] > 191) exit 2
                }
            }
        }
        for (i = 1; i <= n; i++) {
            printf "%d ", b[i]
        }
    }
    ') || status=$?

    if [ "$status" -eq 1 ]; then
        echo "Error: secret file for $label is empty ($file)" >&2
        exit 1
    elif [ "$status" -ne 0 ] || [ -z "$clean_bytes" ]; then
        echo "Error: secret file for $label contains invalid characters" >&2
        exit 1
    fi

    hash_val=$(printf '%s\n' "$clean_bytes" | LC_ALL=C awk '{for (i=1; i<=NF; i++) printf "%c", $i}' | sha256sum | LC_ALL=C awk '{print $1}')
    printf '%s' "$hash_val"
}

ROLE=""
case "$1" in
    streams|cache)
        ROLE="$1"
        shift
        ;;
    *)
        ROLE="${REDIS_ROLE}"
        ;;
esac

ACL_FILE="${REDIS_ACL_FILE:-/tmp/users.acl}"
ACL_DIR=$(dirname "$ACL_FILE")
TMP_ACL=$(mktemp "${ACL_DIR}/.users.acl.tmp.XXXXXX")
chmod 0600 "$TMP_ACL"

case "$ROLE" in
    streams)
        STREAMS_PASS_FILE="${REDIS_STREAMS_PASSWORD_FILE:-/run/secrets/redis_streams_password}"
        LIMITER_PASS_FILE="${REDIS_LIMITER_PASSWORD_FILE:-/run/secrets/redis_limiter_password}"

        STREAMS_HASH=$(get_secret_hash "$STREAMS_PASS_FILE" "streams")
        LIMITER_HASH=$(get_secret_hash "$LIMITER_PASS_FILE" "limiter")

        cat <<EOF > "$TMP_ACL"
user default off -@all
user streams on #${STREAMS_HASH} resetkeys ~alt:events:* ~alt:replies:tags:* -@all +hello +client|setinfo +client|setname +ping +xadd +xread +xreadgroup +xack +xautoclaim +xpending +xgroup|create +xinfo|stream +xinfo|groups +xtrim +expire +ttl +scan +del
user limiter on #${LIMITER_HASH} resetkeys ~host_rate_limiter:v1:* -@all +hello +client|setinfo +client|setname +ping +set +pttl +select
EOF
        ;;

    cache)
        CACHE_PASS_FILE="${REDIS_CACHE_PASSWORD_FILE:-/run/secrets/redis_cache_password}"
        CACHE_HASH=$(get_secret_hash "$CACHE_PASS_FILE" "cache")

        cat <<EOF > "$TMP_ACL"
user default off -@all
user cache on #${CACHE_HASH} resetkeys ~recap_card:* ~recap:summary:* -@all +hello +client|setinfo +client|setname +ping +get +set +del
EOF
        ;;

    *)
        rm -f "$TMP_ACL"
        echo "Error: unknown or unspecified Redis role (must be 'streams' or 'cache')" >&2
        exit 1
        ;;
esac

chmod 0600 "$TMP_ACL"
mv -f "$TMP_ACL" "$ACL_FILE"
chmod 0600 "$ACL_FILE"

exec redis-server --aclfile "$ACL_FILE" "$@"
