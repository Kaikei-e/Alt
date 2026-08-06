import { test, expect, stubURL } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { secureErrorSchema } from "../src/schemas.js";

/**
 * Image surface — the port of `50-images.hurl`.
 *
 * `/v1/images/fetch` pulls an image through the SSRF-guarded fetcher.
 * `/v1/images/proxy/:sig/:url` serves cached OGP thumbnails and is
 * DOS-whitelisted so it can ride alongside SSR pages.
 *
 * Note on the proxy in staging: `IMAGE_PROXY_ENABLED=false`, and
 * `registerImageProxyRoutes` returns before registering anything when the
 * proxy is disabled or its secret is empty. So the route does not exist here
 * at all — what answers the request is the `/v1/images` group's own auth
 * middleware, which Echo runs for unmatched paths under the group prefix.
 * That is why the assertion is 401 and not 404, and why signature validation
 * stays uncovered: there is no secret in staging to sign a valid request with.
 */

test.describe("POST /v1/images/fetch", () => {
	test("rejects a plain-HTTP target", async ({ rest, csrf }) => {
		// image_fetch requires an HTTPS target, so the http:// stub URL is
		// rejected before any outbound request ("only HTTPS URLs are allowed").
		// That makes this a deterministic 400 rather than a fetch test — the
		// stub serves http only, so the fetch path is not reachable from here.
		const body = await expectJsonStatus(
			await rest.post("/v1/images/fetch", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { url: stubURL("image.png") },
			}),
			400,
			secureErrorSchema,
		);
		expect(body.error.code).toBe("VALIDATION_ERROR");
	});

	test("rejects an HTTPS loopback target (SSRF guard)", async ({ rest, csrf }) => {
		// New coverage. The scheme check above short-circuits before the SSRF
		// guard runs, so the Hurl scenario never reached it on this route. An
		// https:// private address is the payload that gets past the first gate
		// and has to be stopped by the second.
		const response = await rest.post("/v1/images/fetch", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: "https://127.0.0.1/image.png" },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});

	test("rejects a missing url", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/images/fetch", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: {},
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("GET /v1/images/proxy/:sig/:url", () => {
	test("sits behind the images group's auth middleware", async ({ restAnon }) => {
		const encoded = encodeURIComponent(stubURL("image.png"));
		await expectStatus(await restAnon.get(`/v1/images/proxy/invalidsig/${encoded}`), 401);
	});

	test("is not registered while the proxy is disabled", async ({ rest }) => {
		// The other half of the same claim, and new coverage: with a valid JWT
		// the group middleware passes and Echo's router has the final word. A
		// 200 here would mean the route got registered without a secret, which
		// di/image_module.go leaves as a nil usecase — i.e. a nil-pointer panic
		// one request away (finding [6]).
		const encoded = encodeURIComponent(stubURL("image.png"));
		const response = await rest.get(`/v1/images/proxy/invalidsig/${encoded}`);
		expect(
			response.status(),
			"IMAGE_PROXY_ENABLED=false in staging, so this route must not resolve",
		).toBe(404);
	});
});
