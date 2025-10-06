# Frontend Unit Tests

- Run the unit suite with `pnpm -C alt-frontend exec vitest run --pool=threads`.
- Set `VITEST_POOL=forks` only when the runtime environment cannot spawn worker threads.
- Control concurrency with `VITEST_MAX_WORKERS` / `VITEST_MAX_CONCURRENCY` when debugging heavy suites.
- Unsupported flags such as `--runInBand` will fail with Vitest v3; rely on the pool settings above instead.
