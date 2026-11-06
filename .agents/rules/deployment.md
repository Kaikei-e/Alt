# Deployment and Container Lifecycle Policy

- **No Unauthorized Deployment**: Never deploy, restart, recreate, start, or stop containers/services (e.g., `docker compose up`, `docker compose restart`, `docker compose down`, `docker stop`, `systemctl`, etc.) without explicit instruction or prior confirmation from the user.
- **Config Edits vs. Applying**: Editing configuration files (such as `compose/*.yaml`, environment variables, configs) is permitted when requested, but applying them to running infrastructure requires explicit user confirmation.
- **Toolchain & Host Environment Boundaries (`LKAL`, `kisage`, `DocDag`)**: Follow root `CLAUDE.md` (`## Toolchain`) and `docs/runbooks/kisage-env.md`. Never edit `.env` or `.kisage/*` directly; verify `deploy/desired-env.lkal` changes with `python3 scripts/kisage-env.py verify` and `docs/ADR/` or `docdag.yaml` changes with `docdag validate && docdag lint --strict`.
