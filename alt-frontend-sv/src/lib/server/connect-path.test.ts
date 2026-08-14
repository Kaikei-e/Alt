import { describe, expect, it } from "vitest";

import { isConnectMethodName, parseConnectPath } from "./connect-path";

describe("isConnectMethodName", () => {
	it.each([
		"Watch",
		"GetFeedStats",
		"Get_v2",
		"a",
		"Stream2",
	])("accepts the proto identifier %s", (value) => {
		expect(isConnectMethodName(value)).toBe(true);
	});

	// The separators below are what SvelteKit hands us after decoding `%5C`,
	// `%2F` and `%2E`, and what `fetch` would then resolve away.
	it.each([
		"",
		"..",
		".",
		"Watch/Extra",
		"Watch\\Extra",
		"..\\..\\v1\\aggregate",
		"../../v1/aggregate",
		"Watch?admin=1",
		"Watch#/v1/aggregate",
		"Watch%2F..%2Fv1",
		"/Watch",
		"2Watch",
		"Watch Extra",
		"Watch\nGET /v1/aggregate",
	])("refuses %j", (value) => {
		expect(isConnectMethodName(value)).toBe(false);
	});
});

describe("parseConnectPath", () => {
	it("splits a well-formed Connect path", () => {
		expect(
			parseConnectPath("alt.knowledge_home.v1.KnowledgeHomeService/Get"),
		).toEqual({
			service: "alt.knowledge_home.v1.KnowledgeHomeService",
			method: "Get",
		});
	});

	// A dot-separated service name must not be mistaken for a traversal-bearing
	// one: only the identifier shape decides, so `..` in either half is out.
	it.each([
		"alt.feeds.v2.FeedService",
		"alt.feeds.v2.FeedService/",
		"alt.feeds.v2.FeedService/Get\\..\\..\\v1\\dashboard",
		"alt.feeds.v2.FeedService/Get/Extra",
		"alt..v2.FeedService/Get",
		"../alt.feeds.v2.FeedService/Get",
		"/GetFeedStats",
	])("refuses %j", (path) => {
		expect(parseConnectPath(path)).toBeNull();
	});
});
