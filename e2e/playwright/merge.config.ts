import { defineConfig } from "@playwright/test";

/**
 * Config for `playwright merge-reports` only — never for running tests.
 *
 * `merge-reports` refuses to merge blob reports recorded with different
 * `testDir`s:
 *
 *     Error: Blob reports being merged were recorded with different test
 *     directories, and merging cannot proceed.
 *
 * That check assumes the usual case — blobs from shards of ONE config — and
 * this fleet is the other case: twelve suites, twelve configs, twelve testDirs
 * (`e2e/playwright/<service>/tests`). The documented escape is a merge config
 * whose `testDir` points at the real location of the tests, which here is the
 * directory every suite sits under.
 *
 * `TestConfig.tag` in each suite's own config is what keeps the blob FILENAMES
 * from colliding when `merge-multiple: true` flattens twelve artifacts into
 * one directory; it does not address testDir, which is a separate check.
 */
export default defineConfig({
	testDir: ".",
});
