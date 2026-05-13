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
});
