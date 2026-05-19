import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const nginxConfigPath = resolve(process.cwd(), "../nginx/conf.d/default.conf");

describe("nginx Connect streaming config", () => {
	it("routes FeedService streaming requests through the dedicated no-buffering location", () => {
		const config = readFileSync(nginxConfigPath, "utf8");

		expect(config).toMatch(
			/location ~ \^\/api\/v2\/alt\\\.\(augur\|morning_letter\|feeds\)\\\.v2\\\.\.\+\/Stream \{/,
		);
		expect(config).toContain("proxy_buffering off;");
		expect(config).toContain("proxy_request_buffering off;");
		expect(config).toContain(
			'add_header Cache-Control "no-cache, no-transform" always;',
		);
	});

	it("uses the WebSocket-safe $connection_upgrade map for /api/ and /", () => {
		const config = readFileSync(nginxConfigPath, "utf8");

		// The map block must exist somewhere above the server { } block so that
		// $connection_upgrade is in scope for the proxy_set_header directives.
		expect(config).toMatch(
			/map\s+\$http_upgrade\s+\$connection_upgrade\s*\{[^}]*default\s+upgrade;[^}]*''\s+close;[^}]*\}/s,
		);

		// Both the /api/ and / locations must use $connection_upgrade (no
		// literal "upgrade" — that would force Connection: upgrade on every
		// normal GET/POST, which Bun's HTTP/1.1 server occasionally trips on
		// and surfaces as "Cannot Open the Page" on iOS Safari).
		// `/ory/` (Kratos) keeps its long-poll-aware literal upgrade per plan.
		const apiLocation =
			/location\s+\/api\/\s*\{[\s\S]*?^\s{4}\}/m.exec(config)?.[0] ?? "";
		expect(apiLocation).toContain(
			"proxy_set_header Connection $connection_upgrade;",
		);
		expect(apiLocation).not.toMatch(
			/proxy_set_header\s+Connection\s+"upgrade";/,
		);

		const rootLocation =
			/location\s+\/\s*\{[\s\S]*?^\s{4}\}/m.exec(config)?.[0] ?? "";
		expect(rootLocation).toContain(
			"proxy_set_header Connection $connection_upgrade;",
		);
		expect(rootLocation).not.toMatch(
			/proxy_set_header\s+Connection\s+"upgrade";/,
		);

		const upgradeRefs =
			config.match(/proxy_set_header\s+Connection\s+\$connection_upgrade;/g) ??
			[];
		expect(upgradeRefs.length).toBeGreaterThanOrEqual(2);
	});

	it("falls back stale /_app/immutable/*.js 404s to the @stale_chunk_reload stub so iOS Safari cannot reach a hard 'Cannot Open the Page'", () => {
		const config = readFileSync(nginxConfigPath, "utf8");

		// A dedicated regex location must intercept JS / MJS chunks so the
		// fallback only fires on script assets (a CSS 404 must not return
		// JS — the browser would silently execute it as CSS, which is just
		// noise but still wrong).
		expect(config).toMatch(
			/location\s+~\*\s+\^\/_app\/immutable\/\.\*\\\.\(js\|mjs\)\$\s*\{/,
		);

		// nginx will not surface upstream 404s through `error_page` unless
		// `proxy_intercept_errors on;` is set explicitly — this is a
		// non-obvious nginx quirk that the next maintainer needs to keep.
		const jsLocation =
			/location\s+~\*\s+\^\/_app\/immutable\/\.\*\\\.\(js\|mjs\)\$\s*\{[\s\S]*?^\s{4}\}/m.exec(
				config,
			)?.[0] ?? "";
		expect(jsLocation).toContain("proxy_intercept_errors on;");
		expect(jsLocation).toMatch(/error_page\s+404\s*=\s*@stale_chunk_reload;/);

		// The named location returning the reload stub must exist and use
		// `application/javascript` so the browser executes the body even
		// though the original request was for a missing JS chunk.
		expect(config).toMatch(/location\s+@stale_chunk_reload\s*\{/);
		const stubLocation =
			/location\s+@stale_chunk_reload\s*\{[\s\S]*?\n\s{4}\}/m.exec(
				config,
			)?.[0] ?? "";
		expect(stubLocation).toContain("default_type application/javascript;");
		expect(stubLocation).toMatch(
			/add_header\s+Cache-Control\s+"no-store"\s+always;/,
		);
		expect(stubLocation).toMatch(
			/add_header\s+X-Stale-Chunk-Reload\s+"1"\s+always;/,
		);
		expect(stubLocation).toMatch(/return\s+200\s+/);
		// The stub body must read & cap the sessionStorage counter and
		// trigger a single location.reload() — otherwise a misconfigured
		// build could loop forever.
		expect(stubLocation).toContain("alt:chunk-reload-attempts");
		expect(stubLocation).toContain("location.reload()");
	});
});
