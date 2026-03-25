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
});
