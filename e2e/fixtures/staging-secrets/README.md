# Staging-only secrets

Files in this directory are **test fixtures**, not production secrets. They
are read exclusively by `compose/compose.staging.yaml` under the
`alt-staging` project name. Do not reference them from any other compose
stack or script.

The values here are committed to the public repo on purpose — they have no
meaning outside the ephemeral E2E network that only exists while Hurl runs
against it.
