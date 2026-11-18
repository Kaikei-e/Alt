import { Code, ConnectError, type Transport } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { getFeedInflowStats, setSubscriptionDailyCap } from "./inflow";

interface RecordedCall {
	method: string;
	input: Record<string, unknown>;
	timeoutMs: number | undefined;
}

function createStubTransport(
	calls: RecordedCall[],
	handler: (methodName: string, input: Record<string, unknown>) => unknown,
): Transport {
	return {
		unary: async (
			method: { name: string; parent: unknown },
			_signal: AbortSignal | undefined,
			timeoutMs: number | undefined,
			_header: HeadersInit | undefined,
			input: unknown,
		) => {
			const record: RecordedCall = {
				method: method.name,
				input: input as Record<string, unknown>,
				timeoutMs,
			};
			calls.push(record);
			const message = handler(method.name, record.input);
			return {
				stream: false as const,
				service: method.parent,
				method,
				header: new Headers(),
				message,
				trailer: new Headers(),
			};
		},
		stream: () => {
			throw new Error("not used");
		},
	} as unknown as Transport;
}

describe("FeedService inflow Connect client", () => {
	describe("getFeedInflowStats", () => {
		it("calls GetFeedInflowStats with 5000ms timeout and converts bigint counts", async () => {
			const calls: RecordedCall[] = [];
			const transport = createStubTransport(calls, () => ({
				stats: [
					{
						feedLinkId: "11111111-1111-4111-8111-111111111111",
						url: "https://example.com/feed.xml",
						dailyCap: 15,
						inflowCount: 420n,
						readCount: 63n,
						openedCountLegacy: 12n,
						readRate: 0.15,
					},
					{
						feedLinkId: "22222222-2222-4222-8222-222222222222",
						url: "https://quiet.example.org/rss",
						dailyCap: undefined,
						inflowCount: 0n,
						readCount: 0n,
						openedCountLegacy: 0n,
						readRate: 0,
					},
				],
				windowDays: 30,
			}));

			const result = await getFeedInflowStats(transport);

			expect(calls).toHaveLength(1);
			expect(calls[0]?.method).toBe("GetFeedInflowStats");
			expect(calls[0]?.timeoutMs).toBe(5000);
			expect(result).toEqual({
				windowDays: 30,
				stats: [
					{
						feedLinkId: "11111111-1111-4111-8111-111111111111",
						url: "https://example.com/feed.xml",
						dailyCap: 15,
						inflowCount: 420,
						readCount: 63,
						openedCountLegacy: 12,
						readRate: 0.15,
					},
					{
						feedLinkId: "22222222-2222-4222-8222-222222222222",
						url: "https://quiet.example.org/rss",
						dailyCap: undefined,
						inflowCount: 0,
						readCount: 0,
						openedCountLegacy: 0,
						readRate: 0,
					},
				],
			});
		});

		it("propagates ConnectError without swallowing", async () => {
			const transport = createStubTransport([], () => {
				throw new ConnectError("backend unavailable", Code.Unavailable);
			});

			await expect(getFeedInflowStats(transport)).rejects.toThrow(
				/backend unavailable/,
			);
		});
	});

	describe("setSubscriptionDailyCap", () => {
		it("sets a positive daily cap with 5000ms timeout", async () => {
			const calls: RecordedCall[] = [];
			const transport = createStubTransport(calls, () => ({
				message: "Daily cap updated",
				dailyCap: 25,
			}));

			const result = await setSubscriptionDailyCap(
				transport,
				"11111111-1111-4111-8111-111111111111",
				25,
			);

			expect(calls).toHaveLength(1);
			expect(calls[0]?.method).toBe("SetSubscriptionDailyCap");
			expect(calls[0]?.timeoutMs).toBe(5000);
			expect(calls[0]?.input).toMatchObject({
				feedLinkId: "11111111-1111-4111-8111-111111111111",
				dailyCap: 25,
			});
			expect(result).toEqual({
				message: "Daily cap updated",
				dailyCap: 25,
			});
		});

		it("omits dailyCap when clearing the cap", async () => {
			const calls: RecordedCall[] = [];
			const transport = createStubTransport(calls, () => ({
				message: "Daily cap cleared",
				dailyCap: undefined,
			}));

			const result = await setSubscriptionDailyCap(
				transport,
				"11111111-1111-4111-8111-111111111111",
			);

			expect(calls).toHaveLength(1);
			expect(calls[0]?.method).toBe("SetSubscriptionDailyCap");
			expect(calls[0]?.timeoutMs).toBe(5000);
			expect(calls[0]?.input.feedLinkId).toBe(
				"11111111-1111-4111-8111-111111111111",
			);
			expect(calls[0]?.input.dailyCap).toBeUndefined();
			expect(result).toEqual({
				message: "Daily cap cleared",
				dailyCap: undefined,
			});
		});

		it("propagates ConnectError on invalid argument", async () => {
			const transport = createStubTransport([], () => {
				throw new ConnectError("invalid feed_link_id", Code.InvalidArgument);
			});

			await expect(
				setSubscriptionDailyCap(transport, "not-a-uuid", 10),
			).rejects.toThrow(/invalid feed_link_id/);
		});
	});

	it("re-exports inflow helpers from connect feeds and index barrels", async () => {
		const feedsBarrel = await import("../feeds");
		const connectBarrel = await import("../index");

		expect(typeof feedsBarrel.getFeedInflowStats).toBe("function");
		expect(typeof feedsBarrel.setSubscriptionDailyCap).toBe("function");
		expect(typeof connectBarrel.getFeedInflowStats).toBe("function");
		expect(typeof connectBarrel.setSubscriptionDailyCap).toBe("function");
	});
});
