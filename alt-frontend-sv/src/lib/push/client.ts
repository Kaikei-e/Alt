/**
 * Browser-side plumbing for push subscriptions: the PushManager half and the
 * Connect-RPC half.
 *
 * Kept out of the component so the page holds state and copy only. The
 * decisions that are worth testing live in `preferences.ts`, `install-state.ts`
 * and `vapid-key.ts`; what remains here is I/O.
 */

import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";

import { createClientTransport } from "$lib/connect";
import {
	NotificationPreferencesSchema,
	PushService,
	PushSubscriptionKeysSchema,
} from "$lib/gen/alt/push/v1/push_pb";

import { noPreferences, type Preferences } from "./preferences";
import { decodeVapidPublicKey } from "./vapid-key";

export interface PushConfig {
	vapidPublicKey: string;
	hasSubscription: boolean;
	preferences: Preferences;
}

function pushClient() {
	return createClient(PushService, createClientTransport());
}

function toProtoPreferences(preferences: Preferences) {
	return create(NotificationPreferencesSchema, {
		summaryReady: preferences.summary_ready,
		acolyteReportReady: preferences.acolyte_report_ready,
		recapReady: preferences.recap_ready,
		todayEntranceReady: preferences.today_entrance_ready,
	});
}

function fromProtoPreferences(
	proto:
		| {
				summaryReady: boolean;
				acolyteReportReady: boolean;
				recapReady: boolean;
				todayEntranceReady: boolean;
		  }
		| undefined,
): Preferences {
	if (!proto) return noPreferences();
	return {
		summary_ready: proto.summaryReady,
		acolyte_report_ready: proto.acolyteReportReady,
		recap_ready: proto.recapReady,
		today_entrance_ready: proto.todayEntranceReady,
	};
}

export async function fetchPushConfig(endpoint: string): Promise<PushConfig> {
	const response = await pushClient().getPushConfig({ endpoint });
	return {
		vapidPublicKey: response.vapidPublicKey,
		hasSubscription: response.hasSubscription,
		preferences: fromProtoPreferences(response.preferences),
	};
}

export async function registerSubscription(
	subscription: PushSubscriptionJSON,
	preferences: Preferences,
): Promise<void> {
	const keys = subscription.keys ?? {};
	await pushClient().registerSubscription({
		endpoint: subscription.endpoint ?? "",
		keys: create(PushSubscriptionKeysSchema, {
			p256dh: keys.p256dh ?? "",
			auth: keys.auth ?? "",
		}),
		preferences: toProtoPreferences(preferences),
	});
}

export async function updatePreferences(
	endpoint: string,
	preferences: Preferences,
): Promise<void> {
	await pushClient().updatePreferences({
		endpoint,
		preferences: toProtoPreferences(preferences),
	});
}

export async function deleteSubscription(endpoint: string): Promise<void> {
	await pushClient().deleteSubscription({ endpoint });
}

/** Whether this browser can subscribe at all, independent of permission. */
export function pushSupported(): boolean {
	return (
		typeof window !== "undefined" &&
		"serviceWorker" in navigator &&
		"PushManager" in window &&
		"Notification" in window
	);
}

/**
 * Must be called synchronously enough from the user's tap to still count as a
 * user gesture — Safari only honours a permission request that originates from
 * one, and silently rejects otherwise.
 */
export async function ensurePermission(): Promise<boolean> {
	if (Notification.permission === "granted") return true;
	if (Notification.permission === "denied") return false;
	return (await Notification.requestPermission()) === "granted";
}

async function pushManager(): Promise<PushManager> {
	const registration = await navigator.serviceWorker.ready;
	return registration.pushManager;
}

export async function currentBrowserSubscription(): Promise<PushSubscriptionJSON | null> {
	if (!pushSupported()) return null;
	const existing = await (await pushManager()).getSubscription();
	return existing ? existing.toJSON() : null;
}

export async function createBrowserSubscription(
	vapidPublicKey: string,
): Promise<PushSubscriptionJSON> {
	const manager = await pushManager();
	const existing = await manager.getSubscription();
	if (existing) return existing.toJSON();

	const subscription = await manager.subscribe({
		// Required by Chrome and Edge, and honest besides: this worker always
		// shows a notification, because Safari revokes permission for a push
		// that does not.
		userVisibleOnly: true,
		applicationServerKey: decodeVapidPublicKey(vapidPublicKey),
	});
	return subscription.toJSON();
}

export async function dropBrowserSubscription(): Promise<void> {
	const existing = await (await pushManager()).getSubscription();
	await existing?.unsubscribe();
}
