import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/paths", () => ({ base: "/sv" }));

let lastTransportConfig: Record<string, unknown> | null = null;
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn((config: Record<string, unknown>) => {
		lastTransportConfig = config;
		return { __transport: true };
	}),
}));

describe("createClientTransport (consolidated $lib/connect/transport-client)", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.clearAllMocks();
		lastTransportConfig = null;
	});

	it("routes through the SvelteKit base path", async () => {
		const { createClientTransport } = await import("./transport-client");

		createClientTransport();

		expect(lastTransportConfig?.baseUrl).toBe("/sv/api/v2");
	});

	it("returns a cached singleton instead of creating a new transport per call", async () => {
		const { createConnectTransport } = await import("@connectrpc/connect-web");
		const { createClientTransport } = await import("./transport-client");

		const first = createClientTransport();
		const second = createClientTransport();

		expect(second).toBe(first);
		expect(createConnectTransport).toHaveBeenCalledTimes(1);
	});

	it("sends credentials so the proxy can forward auth cookies", async () => {
		const { createClientTransport } = await import("./transport-client");
		createClientTransport();

		const fetchFn = lastTransportConfig?.fetch as (
			input: unknown,
			init?: RequestInit,
		) => Promise<Response>;
		const nativeFetch = vi
			.fn()
			.mockResolvedValue(new Response(null, { status: 200 }));
		vi.stubGlobal("fetch", nativeFetch);

		await fetchFn("/api/v2/whatever", { method: "POST" });

		expect(nativeFetch).toHaveBeenCalledWith(
			"/api/v2/whatever",
			expect.objectContaining({ method: "POST", credentials: "include" }),
		);
	});
});

describe("fetch priority sentinel", () => {
	// A sibling describe does not inherit the block above's hooks, and the
	// engine-support probe is memoized per module instance: without this reset
	// the first test's answer decides every later one.
	beforeEach(() => {
		vi.resetModules();
		vi.unstubAllGlobals();
		vi.clearAllMocks();
		lastTransportConfig = null;
	});

	async function callTransportFetch(init?: RequestInit) {
		const { createClientTransport } = await import("./transport-client");
		createClientTransport();

		const fetchFn = lastTransportConfig?.fetch as (
			input: unknown,
			init?: RequestInit,
		) => Promise<Response>;
		const nativeFetch = vi
			.fn()
			.mockResolvedValue(new Response(null, { status: 200 }));
		vi.stubGlobal("fetch", nativeFetch);

		await fetchFn("/api/v2/whatever", init);

		return nativeFetch.mock.calls[0]?.[1] as RequestInit;
	}

	/**
	 * A Request whose constructor reads `priority` out of the init dictionary,
	 * standing in for an engine that implements `RequestInit.priority`. Node's
	 * undici does not, so without this the probe below correctly reports "not
	 * supported" and the assertion under test could never hold.
	 */
	function stubSupportingRequest() {
		vi.stubGlobal(
			"Request",
			class {
				constructor(_input: unknown, init?: Record<string, unknown>) {
					void init?.priority;
				}
			},
		);
	}

	it("turns the sentinel header into RequestInit.priority", async () => {
		stubSupportingRequest();
		const { fetchPriorityHeaders } = await import("./transport-client");

		const init = await callTransportFetch({
			method: "POST",
			headers: new Headers(fetchPriorityHeaders("low")),
		});

		expect((init as { priority?: string }).priority).toBe("low");
	});

	it("carries 'high' through for a request the reader is waiting on", async () => {
		stubSupportingRequest();
		const { fetchPriorityHeaders } = await import("./transport-client");

		const init = await callTransportFetch({
			method: "POST",
			headers: new Headers(fetchPriorityHeaders("high")),
		});

		expect((init as { priority?: string }).priority).toBe("high");
	});

	// The sentinel is an in-process channel between the call site and this
	// transport. Letting it reach the proxy would put a header on the wire
	// that no service declares, that the allowlist never vetted, and that
	// would be forwarded verbatim to alt-backend.
	it("never lets the sentinel reach the network", async () => {
		stubSupportingRequest();
		const { FETCH_PRIORITY_HEADER, fetchPriorityHeaders } = await import(
			"./transport-client"
		);

		const init = await callTransportFetch({
			method: "POST",
			headers: new Headers({
				...fetchPriorityHeaders("low"),
				"content-type": "application/json",
			}),
		});

		const sent = new Headers(init.headers as HeadersInit);
		expect(sent.get(FETCH_PRIORITY_HEADER)).toBeNull();
		expect(sent.get("content-type")).toBe("application/json");
	});

	// Baseline since Chrome 101 / Firefox 132 / Safari 17.2. Older engines
	// ignore unknown init keys, so omitting it is belt-and-braces rather than
	// a polyfill — but the strip must still happen, or the sentinel leaks on
	// exactly the engines that gain nothing from it.
	it("strips the sentinel but sets no priority when the engine lacks it", async () => {
		// No constructor at all: the implicit one never touches the init
		// dictionary, which is exactly what an engine without
		// `RequestInit.priority` looks like to the probe.
		vi.stubGlobal("Request", class {});
		const { FETCH_PRIORITY_HEADER, fetchPriorityHeaders } = await import(
			"./transport-client"
		);

		const init = await callTransportFetch({
			method: "POST",
			headers: new Headers(fetchPriorityHeaders("low")),
		});

		expect("priority" in init).toBe(false);
		expect(
			new Headers(init.headers as HeadersInit).get(FETCH_PRIORITY_HEADER),
		).toBeNull();
	});

	it("leaves a call without the sentinel untouched", async () => {
		const init = await callTransportFetch({ method: "POST" });

		expect("priority" in init).toBe(false);
		expect(init.credentials).toBe("include");
	});
});
