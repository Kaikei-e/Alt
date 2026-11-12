#!/bin/sh
# Exercises entrypoint.sh ACL generation against a stub redis-server.
# shellcheck disable=SC2016,SC2034
set -u

HERE=$(cd "$(dirname "$0")" && pwd)
ENTRYPOINT="$HERE/entrypoint.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/bin"
printf '#!/bin/sh\nexit 0\n' > "$TMP/bin/redis-server"
chmod 0755 "$TMP/bin/redis-server"

printf 'producer_secret\n' > "$TMP/producer"
printf 'consumer_secret\n' > "$TMP/consumer"
printf 'limiter_secret\n' > "$TMP/limiter"
printf 'producer_secret\n' > "$TMP/same"
printf 'reset_secret\n' > "$TMP/reset"

fails=0
check() {
    if ! eval "$2"; then
        echo "FAIL: $1" >&2
        fails=$((fails + 1))
    fi
}

# RESET_USER: value of REDIS_STREAMS_RESET_USER ("__unset__" leaves it unset).
# RESET_FILE: path of the reset secret.
RESET_USER=disabled
RESET_FILE="$TMP/reset"

run_streams() {
    rm -f "$TMP/users.acl"
    (
        if [ "$RESET_USER" = "__unset__" ]; then
            unset REDIS_STREAMS_RESET_USER
        else
            REDIS_STREAMS_RESET_USER="$RESET_USER"
            export REDIS_STREAMS_RESET_USER
        fi
        PATH="$TMP/bin:$PATH" \
        REDIS_ACL_FILE="$TMP/users.acl" \
        REDIS_STREAMS_PRODUCER_PASSWORD_FILE="${1:-$TMP/producer}" \
        REDIS_STREAMS_CONSUMER_PASSWORD_FILE="${2:-$TMP/consumer}" \
        REDIS_LIMITER_PASSWORD_FILE="$TMP/limiter" \
        REDIS_STREAMS_RESET_PASSWORD_FILE="$RESET_FILE" \
        sh "$ENTRYPOINT" streams >"$TMP/out" 2>"$TMP/err"
    )
}

check "streams role boots with both stream secrets" 'run_streams'

producer=$(grep "^user streams_producer " "$TMP/users.acl")
consumer=$(grep "^user streams_consumer " "$TMP/users.acl")
consumer_root=${consumer%%(*}

check "default user is off" 'grep -q "^user default off -@all$" "$TMP/users.acl"'
check "legacy shared streams user is gone" '! grep -q "^user streams " "$TMP/users.acl"'
check "producer user present" '[ -n "$producer" ]'
check "consumer user present" '[ -n "$consumer" ]'
check "producer can append and trim" 'echo "$producer" | grep -q "+xadd" && echo "$producer" | grep -q "+xtrim"'
check "producer cannot read through groups" '! echo "$producer" | grep -q "+xreadgroup"'
check "consumer cannot trim" '! echo "$consumer" | grep -q "+xtrim"'
check "consumer cannot delete" '! echo "$consumer" | grep -Eq "\+del( |$)"'
check "consumer root permissions carry no xadd" '! echo "$consumer_root" | grep -q "+xadd"'
for c in '+xreadgroup' '+xack' '+xautoclaim' '+xpending' '+xgroup|create' '+xinfo|groups'; do
    check "consumer grants $c" 'echo "$consumer" | grep -qF -- "$c"'
done
check "consumer has no xinfo|stream" '! echo "$consumer" | grep -qF "+xinfo|stream"'
producer_root=${producer%%(*}
producer_sel=${producer#*(}
for c in '+xadd' '+xtrim' '+xgroup|create' '+xinfo|stream' '+xinfo|groups' '+ping' '+scan'; do
    check "producer root grants $c" 'echo "$producer_root" | grep -qF -- "$c"'
done
for c in '+del' '+expire' '+ttl' '+xread'; do
    check "producer root does not grant $c" '! echo "$producer_root" | grep -Eq -- "\\$c( |$)"'
    check "producer selector grants $c on reply keys" 'echo "$producer" | grep -qF -- "(~alt:replies:tags:*" && echo "$producer_sel" | grep -qF -- "$c"'
done
check "consumer xadd limited to dead-letter and reply keys" \
    'echo "$consumer" | grep -q "(~alt:events:\*:dlq +xadd)" && echo "$consumer" | grep -q "(~alt:replies:tags:\* +xadd)"'
check "consumer is key-restricted" 'echo "$consumer" | grep -q "resetkeys ~alt:events:\* ~alt:replies:tags:\*"'
check "secrets are hashed, not written" '! grep -q -e producer_secret -e consumer_secret "$TMP/users.acl"'

check "identical producer and consumer secrets are rejected" '! run_streams "$TMP/same" "$TMP/same"'
check "missing consumer secret fails startup" '! run_streams "$TMP/producer" "$TMP/absent"'
check "missing producer secret fails startup" '! run_streams "$TMP/absent" "$TMP/consumer"'

# --- opt-in reset user ------------------------------------------------------
check "disabled mode emits no reset user" '! grep -q "streams_reset" "$TMP/users.acl"'

RESET_USER=__unset__
check "unset REDIS_STREAMS_RESET_USER fails startup" '! run_streams'
check "unset REDIS_STREAMS_RESET_USER error names the variable" 'grep -q "REDIS_STREAMS_RESET_USER" "$TMP/err"'
for bad in yes true ENABLED Enabled ""; do
    RESET_USER=$bad
    check "REDIS_STREAMS_RESET_USER='$bad' fails startup" '! run_streams'
done

RESET_USER=disabled
check "disabled mode boots" 'run_streams'
check "disabled mode says so" 'grep -qi "reset user.*disabled" "$TMP/out" "$TMP/err"'
check "disabled mode keeps exactly the three users" '[ "$(grep -c "^user " "$TMP/users.acl")" -eq 4 ]'
check "disabled mode does not read the reset secret" '(RESET_FILE="$TMP/absent"; run_streams)'

RESET_USER=enabled
check "enabled mode boots" 'run_streams'
check "enabled mode warns" 'grep -qi "reset user.*enabled" "$TMP/out" "$TMP/err"'
reset_hash=$(printf 'reset_secret' | sha256sum | awk '{print $1}')
check "enabled mode writes the exact reset line" \
    'grep -qxF "user streams_reset on #${reset_hash} resetkeys ~alt:events:* -@all +hello +client|setinfo +client|setname +ping +del" "$TMP/users.acl"'
check "enabled mode leaves the other users in place" 'grep -q "^user streams_producer " "$TMP/users.acl" && grep -q "^user streams_consumer " "$TMP/users.acl" && grep -q "^user limiter " "$TMP/users.acl"'
check "reset secret is hashed, not written" '! grep -q reset_secret "$TMP/users.acl"'
check "enabled mode with a missing reset secret fails" '! (RESET_FILE="$TMP/absent"; run_streams)'
for same in producer consumer limiter; do
    check "reset secret equal to the $same secret fails" '! (RESET_FILE="$TMP/$same"; run_streams)'
done
RESET_USER=disabled

if [ "$fails" -ne 0 ]; then
    echo "$fails check(s) failed" >&2
    exit 1
fi
echo "all checks passed"
