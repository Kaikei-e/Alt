# pki-agent/CLAUDE.md

## Overview

Single-responsibility mTLS cert lifecycle sidecar. One instance per east-west
service — **13 subjects** as of ADR-000954:

```
alt-backend      alt-harvester    alt-data-hub     alt-butterfly-facade
auth-hub         pre-processor    search-indexer   tag-generator
recap-worker     recap-subworker  news-creator     rag-orchestrator
acolyte-orchestrator
```

`alt-harvester` and `alt-data-hub` joined in ADR-000954, which split
alt-backend into three binaries: the harvester is an mTLS *client*
(outbox-worker → rag-orchestrator, all jobs → alt-data-hub:9443) and
alt-data-hub is the mTLS *server* for the data plane, whose only listener is
`:9443` with a fail-closed peer allowlist.

The authoritative list is `SUBJECTS` in
`pki-agent/scripts/bootstrap-pki-provisioner.sh`, kept in lockstep with
`EXPECTED_CNS` in `pki-agent/scripts/verify-cn-allowlist.sh` and with the
`pki-agent-*` services in `compose/pki.yaml`. Adding a service means adding it
to the step-ca CN allowlist **first** — otherwise the sidecar cannot mint a
leaf, goes unhealthy, and `depends_on` keeps the whole stack down.

Replaces the brittle compose-embedded `*-cert-init` + `*-cert-renewer` shell
pair.

Responsibility (strictly one): keep `/certs/svc-cert.pem` + `/certs/svc-key.pem`
inside the target volume within its validity window. Period.

## Architecture

Alt-standard Clean Architecture (Handler → Usecase → Port → Gateway → Driver):

```
cmd/pki-agent/main.go                  # wiring + graceful shutdown
internal/
  domain/                              # CertState, sentinel errors, port interfaces
  usecase/rotate.go                    # Tick() state machine (pure)
  adapter/handler/server.go            # /healthz + /metrics
  infrastructure/
    certfile.go                        # atomic write + Load (domain.CertLoader + CertWriter)
    stepca.go                          # step-cli subprocess wrapper (domain.CAIssuer)
    metrics.go                         # Prometheus observer (domain.Observer)
config/config.go                        # env + _FILE secret parsing
```

Usecase layer has zero external imports beyond domain. Infrastructure wraps
step-cli (subprocess) and the OS. Dependency direction: always inward.

## Rotation policy

- `TICK_INTERVAL` default 5m. On every tick, Load → Classify → maybe Issue.
- `RENEW_AT_FRACTION` default 0.66 (Smallstep recommended default).
- Expired cert: ignore `step ca renew` (it needs a valid cert). Re-enroll via
  fresh OTT. New key pair every time — no reuse. (Security audit F-005.)

## Commands

```bash
# Test (TDD first)
go test ./...
go test ./... -race

# Build local binary
go build -o pki-agent ./cmd/pki-agent

# Build image (same base as existing cert sidecars)
docker build -t alt/pki-agent:dev .

# Smoke test against running step-ca
docker run --rm --network alt_alt-network \
  -e STEP_CA_URL=https://step-ca:9000 \
  -e STEP_CA_ROOT_FILE=/trust/ca-bundle.pem \
  -e STEP_CA_PROVISIONER=bootstrap \
  -e STEP_CA_PROVISIONER_PASSWORD_FILE=/run/secrets/step_ca_root_password \
  -e CERT_SUBJECT=pki-agent-smoke -e CERT_SANS=pki-agent-smoke \
  -e CERT_PATH=/tmp/svc-cert.pem -e KEY_PATH=/tmp/svc-key.pem \
  -v alt_pki_trust_bundle:/trust:ro \
  -v $(pwd)/../secrets/step_ca_root_password.txt:/run/secrets/step_ca_root_password:ro \
  alt/pki-agent:dev
```

## Environment variables

| Var | Default | Notes |
|-----|---------|-------|
| STEP_CA_URL | https://step-ca:9000 | internal only |
| STEP_CA_ROOT_FILE | /trust/ca-bundle.pem | published by step-ca-bootstrap |
| STEP_CA_PROVISIONER | pki-agent | dedicated provisioner; bootstrap is fallback |
| STEP_CA_PROVISIONER_PASSWORD_FILE | /run/secrets/step_ca_root_password | Docker secret |
| CERT_SUBJECT | (required) | e.g. alt-backend |
| CERT_SANS | = subject | CSV |
| CERT_PATH | /certs/svc-cert.pem | |
| KEY_PATH | /certs/svc-key.pem | |
| CERT_OWNER_UID | 0 | chown target; 65532 for most, 999 for recap-worker |
| CERT_OWNER_GID | = UID | |
| RENEW_AT_FRACTION | 0.66 | (0,1) |
| TICK_INTERVAL | 5m | Go time.Duration |
| METRICS_ADDR | :9510 | plaintext inside alt-network |
| PROXY_RESPONSE_HEADER_TIMEOUT | 15s | proxy mode only. Wait for the upstream's response headers. Raise it above the caller's own deadline for upstreams that run LLM inference before answering; exceeding it returns 504 |

## Critical rules

1. **TDD first** — failing test before implementation. `go test ./... -race`.
2. **Never reuse keys** — each Issue() call generates a fresh keypair via step-cli.
3. **No renew-after-expiry** — re-enroll with a fresh OTT instead. See security audit F-005.
4. **Atomic writes only** — tmpfile in same dir + rename. chown/chmod before rename.
5. **Provisioner scope** — step-ca's `pki-agent` provisioner has `allowedNames` CN allowlist.

## Prometheus metrics

- `pki_agent_cert_not_after_seconds{subject}` — gauge, unix ts
- `pki_agent_cert_remaining_seconds{subject}` — gauge, seconds
- `pki_agent_last_rotation_timestamp_seconds{subject}` — gauge
- `pki_agent_renewal_total{subject, result}` — counter
- `pki_agent_reissue_total{subject, reason}` — counter
- `pki_agent_up{subject}` — gauge 1
- `pki_agent_healthy{subject}` — gauge 1/0

Scraped by Prometheus inside alt-network via `pki-agent-<svc>:9510/metrics`.
