/**
 * Push API Contract Tests
 *
 * Validates `alt.push.v1` proto schema conformance from the frontend side.
 *
 * These are schema-conformance tests, not consumer-driven contracts:
 * `alt-frontend-sv` is registered as `kind: runtime` in `services.yaml` and is
 * deliberately not a Pact pacticipant. What they buy is that a field rename or
 * type change in the proto breaks here — at compile time — rather than at
 * runtime in a browser.
 */
import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";

import {
	GetPushConfigRequestSchema,
	GetPushConfigResponseSchema,
	NotificationPreferencesSchema,
	PushSubscriptionKeysSchema,
	RegisterSubscriptionRequestSchema,
	UpdatePreferencesRequestSchema,
} from "$lib/gen/alt/push/v1/push_pb";

describe("Push API Contract", () => {
	describe("NotificationPreferences", () => {
		it("carries one flag per notification kind", () => {
			const preferences = create(NotificationPreferencesSchema, {
				summaryReady: true,
				acolyteReportReady: false,
				recapReady: true,
				todayEntranceReady: false,
			});

			expect(preferences.summaryReady).toBe(true);
			expect(preferences.acolyteReportReady).toBe(false);
			expect(preferences.recapReady).toBe(true);
			expect(preferences.todayEntranceReady).toBe(false);
		});

		it("defaults every kind to off", () => {
			// proto3 scalar defaults mean an absent preferences block reads as
			// "nothing enabled" rather than as "everything enabled".
			const preferences = create(NotificationPreferencesSchema, {});

			expect(preferences.summaryReady).toBe(false);
			expect(preferences.acolyteReportReady).toBe(false);
			expect(preferences.recapReady).toBe(false);
			expect(preferences.todayEntranceReady).toBe(false);
		});
	});

	describe("RegisterSubscriptionRequest", () => {
		it("carries the endpoint and both RFC 8291 key fields", () => {
			const request = create(RegisterSubscriptionRequestSchema, {
				endpoint: "https://fcm.googleapis.com/fcm/send/abc123",
				keys: create(PushSubscriptionKeysSchema, {
					p256dh:
						"BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM",
					auth: "tBHItJI5svbpez7KI4CCXg",
				}),
				preferences: create(NotificationPreferencesSchema, {
					recapReady: true,
				}),
			});

			expect(request.endpoint).toContain("fcm.googleapis.com");
			expect(request.keys?.p256dh.startsWith("B")).toBe(true);
			expect(request.keys?.auth).toBe("tBHItJI5svbpez7KI4CCXg");
			expect(request.preferences?.recapReady).toBe(true);
		});

		it("round-trips through proto serialization", () => {
			const original = create(RegisterSubscriptionRequestSchema, {
				endpoint: "https://web.push.apple.com/xyz",
				keys: create(PushSubscriptionKeysSchema, {
					p256dh: "BKey",
					auth: "AAuth",
				}),
				preferences: create(NotificationPreferencesSchema, {
					todayEntranceReady: true,
				}),
			});

			const restored = fromBinary(
				RegisterSubscriptionRequestSchema,
				toBinary(RegisterSubscriptionRequestSchema, original),
			);

			expect(restored.endpoint).toBe("https://web.push.apple.com/xyz");
			expect(restored.keys?.p256dh).toBe("BKey");
			expect(restored.preferences?.todayEntranceReady).toBe(true);
		});
	});

	describe("GetPushConfig", () => {
		it("accepts an empty endpoint for a browser with no subscription yet", () => {
			const request = create(GetPushConfigRequestSchema, { endpoint: "" });

			expect(request.endpoint).toBe("");
		});

		it("returns the VAPID public key plus the stored state", () => {
			const response = create(GetPushConfigResponseSchema, {
				vapidPublicKey: "BA1Hxzyi1RUM1b5wjxsn7nGxAszw",
				hasSubscription: true,
				preferences: create(NotificationPreferencesSchema, {
					recapReady: true,
				}),
			});

			expect(response.vapidPublicKey).toBeTruthy();
			expect(response.hasSubscription).toBe(true);
			expect(response.preferences?.recapReady).toBe(true);
		});

		it("omits preferences when there is no subscription", () => {
			const response = create(GetPushConfigResponseSchema, {
				vapidPublicKey: "BA1Hxzyi1RUM1b5wjxsn7nGxAszw",
				hasSubscription: false,
			});

			// The client must treat an absent block as "all off" rather than
			// reading undefined flags as enabled.
			expect(response.preferences).toBeUndefined();
		});
	});

	describe("UpdatePreferencesRequest", () => {
		it("is keyed by endpoint, because settings are per device", () => {
			const request = create(UpdatePreferencesRequestSchema, {
				endpoint: "https://fcm.googleapis.com/fcm/send/one-device",
				preferences: create(NotificationPreferencesSchema, {
					summaryReady: true,
				}),
			});

			expect(request.endpoint).toContain("one-device");
			expect(request.preferences?.summaryReady).toBe(true);
		});
	});
});
