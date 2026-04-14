#!/bin/sh
set -eu

: "${UPSTREAM_PORT:?UPSTREAM_PORT must be set}"

# VERIFY_CLIENT controls enforcement of peer mTLS at the nginx sidecar.
#   off (default): server cert only, parity with Phase 1 dual-stack
#   on: require a valid client cert signed by /trust/ca-bundle.pem
case "${VERIFY_CLIENT:-off}" in
  on)
    VERIFY_CLIENT_BLOCK="ssl_verify_client on; ssl_client_certificate /trust/ca-bundle.pem; ssl_verify_depth 2;"
    ;;
  *)
    VERIFY_CLIENT_BLOCK=""
    ;;
esac
export VERIFY_CLIENT_BLOCK

envsubst '${UPSTREAM_PORT} ${VERIFY_CLIENT_BLOCK}' </etc/nginx/nginx.conf.template >/tmp/nginx.conf

for i in $(seq 1 30); do
  if nc -z 127.0.0.1 "$UPSTREAM_PORT" 2>/dev/null; then
    break
  fi
  echo "nginx-tls-sidecar: waiting for upstream 127.0.0.1:${UPSTREAM_PORT} (attempt ${i})"
  sleep 1
done

exec nginx -c /tmp/nginx.conf -e /dev/stderr
