---
title: PostgreSQL AppArmor Confinement on Ubuntu 26.04 (alt-docker-postgres)
date: 2026-11-05
tags:
  - runbook
  - security
  - apparmor
  - postgres
  - docker
---
# PostgreSQL AppArmor Confinement on Ubuntu 26.04 (`alt-docker-postgres`)

## 1. Problem & Root Cause

On Ubuntu 26.04 hosts (`kernel.apparmor_restrict_unprivileged_unconfined=1`), `runc` runs unconfined (or under Ubuntu's named-unconfined `runc` profile) without `CAP_MAC_OVERRIDE` when executing `aa_change_onexec` into a container's AppArmor profile. The kernel converts the transition into a **stacked AppArmor label**:

```text
docker-default//&unconfined
```

Docker Engine's built-in `docker-default` template (`moby/profiles/apparmor/template.go`) only permits intra-container signals for the unstacked peer name (`signal (send,receive) peer=docker-default,`). When PostgreSQL's postmaster signals its child processes (`checkpointer`, `bgwriter`, `walwriter`, `autovacuum`) or `gosu` signals during entrypoint initialization, the kernel evaluates the peer as `docker-default//&unconfined` and denies the signal with `EACCES` (`Permission denied`).

Upstream references:
- [containerd/containerd#14485](https://github.com/containerd/containerd/issues/14485) and [containerd/containerd#14487](https://github.com/containerd/containerd/pull/14487)
- [Docker Engine AppArmor Documentation](https://docs.docker.com/engine/security/apparmor/)

---

## 2. Custom Profile Design (`deploy/apparmor/alt-docker-postgres`)

`deploy/apparmor/alt-docker-postgres` retains 15 filesystem and mount restrictions from the Docker-style baseline (`@{PROC}`, `/sys`, `deny mount,`), blocks `AF_ALG` and `AF_VSOCK`, and scopes `ptrace` to `peer=alt-docker-postgres`. It adds exact stacked `signal` peers. It is a maintained custom policy, not an automatically synchronized copy of every future Docker policy update:

- **Inbound container-runtime stop/kill signals**: `peer=unconfined`, `peer=runc` (Ubuntu's named-unconfined `runc` profile, required for `docker stop`), `peer=dockerd`, `peer=/usr/bin/dockerd`.
  - *Daemon-profile variability note*: If a host runs `dockerd` under a non-standard AppArmor profile (such as a Snap profile or custom path), add that daemon profile name to the `signal (receive) peer=...` list in `deploy/apparmor/alt-docker-postgres`.
- **Intra-container signals**: `peer=alt-docker-postgres`, `peer=alt-docker-postgres//&unconfined`, and `peer=unconfined//&alt-docker-postgres`.
- **No global host weakening**: Does not modify `kernel.apparmor_restrict_unprivileged_unconfined` or overwrite `docker-default`.

---

## 3. Host Prerequisite & OSS Installation Procedure

> [!IMPORTANT]
> **Linux AppArmor-Specific Host Prerequisite (Fail-Closed by Default)**
> All 8 production PostgreSQL services (`compose/db.yaml`, `compose/auth.yaml`, `compose/acolyte.yaml`, `compose/pact.yaml`, `compose/rag.yaml`, `compose/recap.yaml`, `compose/sovereign.yaml`) set `security_opt: [apparmor:alt-docker-postgres]` by default, with no `apparmor:unconfined` fallback and no automatic privileged installation. `compose/compose.staging.yaml` leaves `security_opt` unset on its 6 staging PostgreSQL services so portable CI runners (`ubuntu-latest`) run under Docker's built-in `docker-default` AppArmor profile, while `compose/compose.staging.apparmor.yaml` provides the explicit `apparmor:alt-docker-postgres` override for Ubuntu 26.04 hosts.
>
> When using the production stack or the staging AppArmor override on Linux hosts with AppArmor enabled, `/etc/apparmor.d/alt-docker-postgres` **must** be installed and loaded into the kernel before the first `docker compose up`. If the selected profile is absent or unloaded, Docker Engine fails closed at container start. This prerequisite is specific to Linux hosts running AppArmor (validated on Ubuntu 26.04); cross-platform or non-AppArmor host behavior is not claimed as tested here.

### Step 1: Validate, Install, and Load the Profile (Minimal-Privilege Sequence)

Run the unprivileged policy compile check first, then use `sudo` only for the file installation and kernel load, followed by the unprivileged verification script:

```bash
# 1. Unprivileged syntax & policy compile check (-K avoids root-owned /var/cache/apparmor)
apparmor_parser --preprocess -K deploy/apparmor/alt-docker-postgres >/dev/null &&
apparmor_parser -Q -K deploy/apparmor/alt-docker-postgres >/dev/null &&

# 2. Install policy file as root:root 0644
sudo /usr/bin/install -o root -g root -m 0644 \
  deploy/apparmor/alt-docker-postgres /etc/apparmor.d/alt-docker-postgres &&

# 3. Load/replace policy into kernel and write cache (-r -W -T reads from file, skips cache read)
sudo /usr/sbin/apparmor_parser -r -W -T /etc/apparmor.d/alt-docker-postgres &&

# 4. Run unprivileged static + runtime container signal probe
./scripts/check-apparmor-profile.sh --probe
```

Alternatively, `sudo ./scripts/install-apparmor-profile.sh` wraps the pre-install `-Q -K` validation, `0644` installation, `-r -W` kernel reload, and `scripts/check-apparmor-profile.sh --probe` invocation.

### Step 2: Non-Root Preflight & Runtime Probe Verification

```bash
./scripts/check-apparmor-profile.sh --probe
```

What `scripts/check-apparmor-profile.sh` and `scripts/probe-apparmor-postgres.sh` verify:
- Confirms `/etc/apparmor.d/alt-docker-postgres` exists, is readable, matches `deploy/apparmor/alt-docker-postgres` byte-for-byte, and compiles cleanly with `apparmor_parser --preprocess -K` and `apparmor_parser -Q -K`.
- Starts a bounded-timeout ephemeral container (`--pull=never --network=none --user postgres --security-opt apparmor=alt-docker-postgres postgres:17.11-alpine`).
- Asserts `/proc/self/attr/apparmor/current` is `alt-docker-postgres (enforce)` or `alt-docker-postgres//&unconfined (enforce)` (rejecting `unconfined` or `docker-default`).
- Delivers `SIGUSR1` from the parent shell to a distinct child process and verifies actual signal receipt.

---

## 4. Default Compose Enforcement, Rollout Verification & Compatibility Overrides

### Default Compose Usage
Production Compose files select `apparmor:alt-docker-postgres` directly:

- **Production stack** (8 PostgreSQL services: `db`, `pre-processor-db`, `kratos-db`, `acolyte-db`, `pact-db`, `rag-db`, `recap-db`, `knowledge-sovereign-db`, declared in `deploy/host-prereqs.yaml` under `host_facts.apparmor_postgres_profile`):
  ```bash
  ./scripts/check-apparmor-profile.sh --probe
  docker compose -f compose/compose.yaml -p alt up -d
  ```
- **Staging E2E stack** (6 PostgreSQL services: `knowledge-sovereign-db`, `rag-db`, `recap-db`, `acolyte-db`, `alt-backend-db`, `auth-hub-db` use Docker's built-in `docker-default` confinement on AppArmor-enabled CI hosts):
  ```bash
  docker compose -f compose/compose.staging.yaml --profile <suite> -p alt-staging up -d --wait
  ```
- **Staging on Ubuntu 26.04 hosts requiring the custom profile**, after provisioning it:
  ```bash
  ./scripts/check-apparmor-profile.sh --probe
  docker compose -f compose/compose.staging.yaml -f compose/compose.staging.apparmor.yaml --profile <suite> -p alt-staging up -d --wait
  ```

### Override Files (`compose/compose.apparmor.yaml` & `compose/compose.staging.apparmor.yaml`)
`compose/compose.apparmor.yaml` is retained as a compatibility override (`security_opt: !override`) for production callers that pass `-f compose/compose.apparmor.yaml`, and `compose/compose.staging.apparmor.yaml` applies `apparmor:alt-docker-postgres` (`security_opt: !override`) to the 6 staging PostgreSQL services on Ubuntu 26.04 hosts while keeping `compose/compose.staging.yaml` portable on standard CI runners.

### Validation Status & Host Rollout Verification Checklist

The profile explicitly selects ABI 3.0 for Unix socket compatibility and denies `AF_ALG` and `AF_VSOCK`, following the [Moby profile template](https://github.com/moby/profiles/blob/main/apparmor/template.go).

- **Validated on local host**: `/etc/apparmor.d/alt-docker-postgres` is installed and loaded into the kernel, and `./scripts/check-apparmor-profile.sh --probe` passes with runtime label `alt-docker-postgres//&unconfined (enforce)` and verified intra-container `SIGUSR1` delivery.
- **Production DB rollout verified on this host (2026-11-05)**: All 8 running PostgreSQL services were recreated sequentially with the same image IDs and named volumes. Each became healthy with `alt-docker-postgres//&unconfined (enforce)`, completed SQL `CHECKPOINT`, reported `pg_is_in_recovery() = false` and `autovacuum = on`, and had an active autovacuum launcher. This verifies launcher availability, not completion of an autovacuum job on every table. A separate short-lived container handled Docker's stop signal and exited with code 0 under the custom profile.
- **Transition behavior**: Some old `docker-default` containers could not finish fast shutdown within the 120-second grace period. Docker then stopped them forcibly and PostgreSQL performed WAL recovery on startup. Recovery and subsequent `CHECKPOINT` completed successfully. Keep the rollout sequential and verify recovery before proceeding to the next DB.
- **Other environments**: Staging Compose configuration was rendered successfully; no staging stack or additional Docker host was started as part of this rollout. Install and verify the profile independently on each additional host before starting its PostgreSQL services.

When rolling out or restarting PostgreSQL containers on a host, verify each database service in order:

```bash
# 1. Confirm the named profile is loaded in enforce mode on the host
./scripts/check-apparmor-profile.sh --probe

# 2. Recreate the target PostgreSQL service(s)
docker compose -f compose/compose.yaml -p alt up -d --no-deps --no-build --pull never --timeout 120 --wait --wait-timeout 180 db

# 3. Verify the running container's effective AppArmor label is confined (enforce)
docker compose -f compose/compose.yaml -p alt exec -T db \
  cat /proc/1/attr/apparmor/current
# Expected: alt-docker-postgres (enforce) or alt-docker-postgres//&unconfined (enforce)

# 4. Verify PostgreSQL readiness and postmaster/checkpointer IPC via CHECKPOINT
docker compose -f compose/compose.yaml -p alt exec -T --user postgres db \
  sh -c 'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -c "CHECKPOINT;"'

# 5. Verify clean container stop/signal handling (runc/dockerd receive rule) when testing lifecycle
docker compose -f compose/compose.yaml -p alt stop db
docker compose -f compose/compose.yaml -p alt start db
```

---

## 5. Optional External Deployment Pipeline Integration

OSS users do not need any external or private deployment repository; `deploy/host-prereqs.yaml`, `deploy/apparmor/alt-docker-postgres`, and `scripts/check-apparmor-profile.sh` are self-contained in this repository.

For environments that use an external deployment pipeline or self-hosted runners (such as `alt-prod` and `alt-builder` with `RUNNER_ALLOW_RUNASROOT=0`):
1. Provision `/etc/apparmor.d/alt-docker-postgres` once on each runner host using the Step 1 commands above.
2. Add an unprivileged preflight step running `./scripts/check-apparmor-profile.sh --probe` before `docker compose up` in deploy or E2E workflows so missing or drifted host profiles fail fast before container recreation.

---

## 6. Rollback (Retaining AppArmor Confinement)

Do **not** revert PostgreSQL services to `apparmor:unconfined` as a normal rollback procedure. If a rollout or policy update encounters an issue while maintaining confinement:

1. **Pause rollout and safely stop affected DB containers**:
   Stop the rollout before recreating remaining database services. If an affected PostgreSQL container must be quiesced:
   ```bash
   docker compose -f compose/compose.yaml -p alt stop <db-service>
   ```
2. **Restore the last known-good confined policy**:
   Check out the last known-good revision of `deploy/apparmor/alt-docker-postgres`, validate with `apparmor_parser -Q -K`, reinstall to `/etc/apparmor.d/alt-docker-postgres`, reload with `sudo /usr/sbin/apparmor_parser -r -W -T /etc/apparmor.d/alt-docker-postgres`, and confirm `./scripts/check-apparmor-profile.sh --probe` passes before starting the container again:
   ```bash
   docker compose -f compose/compose.yaml -p alt up -d <db-service>
   ```
3. **Host profile removal (decommissioned hosts only)**:
   Only when decommissioning a host after all containers using `alt-docker-postgres` have been permanently stopped and removed:
   ```bash
   sudo /usr/sbin/apparmor_parser -R /etc/apparmor.d/alt-docker-postgres
   sudo rm -f /etc/apparmor.d/alt-docker-postgres
   ```
