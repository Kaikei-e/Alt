import { createConnectTransport } from "@connectrpc/connect-web";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { stubFetch, mockCreateKeepaliveTransport, mockCreateClientTransport } =
	vi.hoisted(() => {
		const stubFetch = vi.fn(
			async () =>
				new Response(JSON.stringify({}), {
					status: 200,
					headers: { "Content-Type": "application/json" },
				}),
		);
		const mockCreateKeepaliveTransport = vi.fn(() =>
			createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			}),
		);
		const mockCreateClientTransport = vi.fn(() =>
			createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			}),
		);
		return {
			stubFetch,
			mockCreateKeepaliveTransport,
			mockCreateClientTransport,
		};
	});

vi.mock("#lib/connect/reading-transport.js", () => ({
	createKeepaliveTransport: mockCreateKeepaliveTransport,
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: mockCreateClientTransport,
}));

import {
	getTheme,
	listThemes,
	recordThemeOpened,
} from "#lib/connect/reading.js";

describe("Theme transport selection", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("recordThemeOpened uses the keepalive-capable transport when transport is omitted", async () => {
		await recordThemeOpened("01938e82-7c00-7a7b-9b10-100000000001", 2);
		expect(mockCreateKeepaliveTransport).toHaveBeenCalledTimes(1);
		expect(mockCreateClientTransport).not.toHaveBeenCalled();
	});

	it("listThemes uses the standard client transport by default", async () => {
		stubFetch.mockResolvedValueOnce(
			new Response(JSON.stringify({ themes: [] }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		await listThemes();
		expect(mockCreateClientTransport).toHaveBeenCalledTimes(1);
		expect(mockCreateKeepaliveTransport).not.toHaveBeenCalled();
	});

	it("getTheme uses the standard client transport by default", async () => {
		stubFetch.mockResolvedValueOnce(
			new Response(
				JSON.stringify({
					themeId: "01938e82-7c00-7a7b-9b10-100000000001",
					title: "Compiler Toolchains",
					version: 1,
					latestVersion: 1,
					keyPoints: [],
					timeline: [],
					whatChanged: [],
					openQuestions: [],
					sources: [],
					versions: [],
				}),
				{
					status: 200,
					headers: { "Content-Type": "application/json" },
				},
			),
		);
		await getTheme("01938e82-7c00-7a7b-9b10-100000000001");
		expect(mockCreateClientTransport).toHaveBeenCalledTimes(1);
		expect(mockCreateKeepaliveTransport).not.toHaveBeenCalled();
	});
});
