# pki-agent/CLAUDE.md

## Overview

**Tooling only.** Compose workload sidecars are **0**. Leaf issue/renew is
in-process on **14 parents** (`PKI_ENROLLMENT=enabled`, [[000978]]). Do not
re-add `pki-agent-*` services — a leftover container on the same cert volume
is a **dual writer**.

The 14 parents:

```
alt-backend          alt-harvester           alt-data-hub
alt-notifier         alt-butterfly-facade    auth-hub
pre-processor        search-indexer          tag-generator
recap-worker         recap-subworker         news-creator
rag-orchestrator     acolyte-orchestrator
```

`compose/pki.yaml` is **step-ca + step-ca-bootstrap** only. This directory
keeps the Go binary, image, and bootstrap / CN-allowlist scripts so
operators can mint emergency leaves and provision subject-scoped JWKs.

The authoritative CN list is `SUBJECTS` in
`pki-agent/scripts/bootstrap-pki-provisioner.sh`, kept in lockstep with
`EXPECTED_CNS` in `pki-agent/scripts/verify-cn-allowlist.sh` and with the
14 in-process parents. Adding a service means adding it to the step-ca CN
allowlist **first**.

Live Prometheus scrape is parent `:9110` `pki_enrollment_*` (unpublished
ops). `:9510` / `pki_agent_*` are the **historical sidecar surface**, not
the current scrape job.

Replaces the brittle compose-embedded `*-cert-init` + `*-cert-renewer` shell
pair (and later the 14 workload sidecars).

Responsibility of the remaining **tooling** binary: keep
`/certs/svc-cert.pem` + `/certs/svc-key.pem` inside a target volume within
its validity window when an operator runs it by hand (emergency mint).
Period. Production writers are the 14 parents.

## Architecture

Alt-standard Clean Architecture (Handler → Usecase → Port → Gateway → Driver):

```
cmd/pki-agent/main.go                  # wiring + graceful shutdown
internal/
  domain/                              # CertState, sentinel errors, port interfaces
  usecase/rotate.go                    # Tick() state machine (pure)
  adapter/handler/server.go            # /healthz + /metrics (tooling :9510)
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

# Build tooling image (not a compose workload)
docker build -t alt/pki-agent:dev .

# Smoke test against running step-ca (emergency / bootstrap only)
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
| STEP_CA_PROVISIONER | pki-agent-\<CERT_SUBJECT\> | per-subject JWK. Shared name `pki-agent` is forbidden for workload enrollment |
| STEP_CA_PROVISIONER_PASSWORD_FILE | /run/secrets/pki-agent-\<CERT_SUBJECT\>-jwk | per-subject Docker secret; never `step_ca_root_password` |
| CERT_SUBJECT | (required) | e.g. alt-backend |
| CERT_SANS | = subject | CSV |
| CERT_PATH | /certs/svc-cert.pem | |
| KEY_PATH | /certs/svc-key.pem | |
| CERT_OWNER_UID | 0 | chown target; 65532 for most, 999 for recap-worker |
| CERT_OWNER_GID | = UID | |
| RENEW_AT_FRACTION | 0.66 | (0,1) |
| TICK_INTERVAL | 5m | Go time.Duration |
| METRICS_ADDR | :9510 | tooling / historical sidecar listen. Live scrape is parent `:9110` |
| PROXY_RESPONSE_HEADER_TIMEOUT | 15s | proxy mode only. Historical; inbound TLS is in the parent |

## Critical rules

1. **TDD first** — failing test before implementation. `go test ./... -race`.
2. **Never reuse keys** — each Issue() call generates a fresh keypair via step-cli.
3. **No renew-after-expiry** — re-enroll with a fresh OTT instead. See security audit F-005.
4. **Atomic writes only** — tmpfile in same dir + rename. chown/chmod before rename.
5. **Provisioner scope** — each CERT_SUBJECT has its own JWK (`pki-agent-<subject>`) and password file. Authority-level CN allowlist still applies. Do not ship the shared root/JWK password into a workload.

## Prometheus metrics

Live (14 parents, `:9110/metrics`):

- `pki_enrollment_healthy{subject}` and siblings — scraped by Prometheus.
  Absence pages via `absent()`.

Historical sidecar surface (`pki_agent_*` on `:9510`) is **not** a current
scrape job. Leftover `pki-agent-*` containers are dual writers; detect them
with `docker ps` labels, not Prometheus.
