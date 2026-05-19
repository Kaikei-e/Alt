/**
 * The server hook must stamp every HTML SSR response with
 * `Cache-Control: no-cache, must-revalidate` so the browser revalidates
 * the HTML on every navigation. Otherwise the user keeps a stale HTML
 * (with old `_app/immutable/*` script tags) and races a deploy.
 *
 * Vite's "Load Error Handling" docs explicitly state:
 *   "make sure to set Cache-Control: no-cache on the HTML file,
 *    otherwise the old assets will be still referenced."
 *
 * The Cache-Control must NOT be set on non-HTML responses (Connect-RPC
 * JSON, static JS chunks under /_app/immutable/, image proxy bytes, …)
 * — those rely on their own cache headers (`public, immutable` or
 * Connect's own caching directives).
 */

import { describe, expect, it, vi } from "vitest";

import { applyHtmlCacheControl } from "./hooks.server.cache-control";

function htmlResponse() {
	return new Response("<!doctype html><html></html>", {
		headers: { "content-type": "text/html; charset=utf-8" },
	});
}

function jsonResponse() {
	return new Response('{"ok":true}', {
		headers: { "content-type": "application/json" },
	});
}

function jsResponse() {
	return new Response("export const x = 1;", {
		headers: { "content-type": "application/javascript" },
	});
}

describe("applyHtmlCacheControl", () => {
	it("stamps Cache-Control: no-cache, must-revalidate on text/html responses", () => {
		const res = htmlResponse();
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBe("no-cache, must-revalidate");
	});

	it("leaves JSON responses untouched (Connect-RPC must keep its own caching)", () => {
		const res = jsonResponse();
		const before = res.headers.get("cache-control");
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBe(before);
	});

	it("leaves application/javascript responses untouched (immutable chunks)", () => {
		const res = jsResponse();
		const before = res.headers.get("cache-control");
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBe(before);
	});

	it("handles missing content-type header gracefully", () => {
		const res = new Response("anything");
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBeNull();
	});

	it("matches charset-suffixed text/html (text/html; charset=utf-8)", () => {
		const res = new Response("<html></html>", {
			headers: { "content-type": "text/html; charset=UTF-8" },
		});
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBe("no-cache, must-revalidate");
	});

	it("overwrites any pre-existing Cache-Control on HTML responses (definitive set)", () => {
		const res = new Response("<html></html>", {
			headers: {
				"content-type": "text/html",
				"cache-control": "public, max-age=3600",
			},
		});
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBe("no-cache, must-revalidate");
	});

	it("does not throw if the Response headers are read-only by spec — soft fail tolerated", () => {
		// In normal Node fetch Response headers are mutable; this guard is a
		// regression catch for environments that freeze headers.
		const res = htmlResponse();
		const frozen = Object.freeze(res);
		expect(() => applyHtmlCacheControl(frozen)).not.toThrow();
	});

	it("is idempotent — calling twice yields the same header", () => {
		const res = htmlResponse();
		applyHtmlCacheControl(res);
		applyHtmlCacheControl(res);
		expect(res.headers.get("cache-control")).toBe("no-cache, must-revalidate");
	});

	it("only inspects content-type, not the body (no body re-read)", () => {
		const stream = new ReadableStream<Uint8Array>({
			start(controller) {
				controller.enqueue(new TextEncoder().encode("<html></html>"));
				controller.close();
			},
		});
		const spy = vi.spyOn(stream, "getReader");
		const res = new Response(stream, {
			headers: { "content-type": "text/html" },
		});
		applyHtmlCacheControl(res);
		expect(spy).not.toHaveBeenCalled();
		expect(res.headers.get("cache-control")).toBe("no-cache, must-revalidate");
	});
});
