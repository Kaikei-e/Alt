<script lang="ts">
/**
 * Notification settings.
 *
 * Single layout rather than the desktop/mobile fork used by `settings/feeds`:
 * the page is one column of labelled switches, so duplicating the markup would
 * buy nothing and double the places a copy change has to land.
 *
 * The toggle is never optimistic. A control that flips immediately and then
 * quietly fails leaves the user believing notifications are on while nothing is
 * subscribed — the exact silent failure this feature is trying not to
 * reproduce, and the reason there is a test for the failing path.
 */
import { onMount } from "svelte";

import { Switch } from "$lib/components/ui/switch";
import {
	createBrowserSubscription,
	currentBrowserSubscription,
	deleteSubscription,
	dropBrowserSubscription,
	ensurePermission,
	fetchPushConfig,
	pushSupported,
	registerSubscription,
	updatePreferences,
} from "$lib/push/client";
import { isInstalled } from "$lib/push/install-state";
import {
	decideAction,
	KIND_DESCRIPTIONS,
	KIND_LABELS,
	NOTIFICATION_KINDS,
	type NotificationKind,
	noPreferences,
	type Preferences,
} from "$lib/push/preferences";

let preferences = $state<Preferences>(noPreferences());
let hasSubscription = $state(false);
let vapidPublicKey = $state("");
let endpoint = $state("");
let installed = $state(true);
let supported = $state(true);
let loading = $state(true);
let busyKind = $state<NotificationKind | null>(null);
let errorText = $state<string | null>(null);

function testId(kind: NotificationKind): string {
	return `notification-toggle-${kind.replace(/_/g, "-")}`;
}

onMount(async () => {
	supported = pushSupported();
	installed = isInstalled(window);

	try {
		const existing = supported ? await currentBrowserSubscription() : null;
		endpoint = existing?.endpoint ?? "";

		const config = await fetchPushConfig(endpoint);
		vapidPublicKey = config.vapidPublicKey;
		hasSubscription = config.hasSubscription;
		preferences = config.preferences;
	} catch {
		errorText = "Could not load your notification settings.";
	} finally {
		loading = false;
	}
});

async function setKind(kind: NotificationKind, next: boolean) {
	if (busyKind !== null) return;

	busyKind = kind;
	errorText = null;

	const desired: Preferences = { ...preferences, [kind]: next };

	try {
		switch (decideAction(hasSubscription, desired)) {
			case "subscribe": {
				// Permission first, and early: Safari only honours a request that
				// still counts as originating from the user's tap.
				if (!(await ensurePermission())) {
					errorText =
						"Notifications are blocked for this site in your browser settings.";
					return;
				}
				const subscription = await createBrowserSubscription(vapidPublicKey);
				await registerSubscription(subscription, desired);
				endpoint = subscription.endpoint ?? "";
				hasSubscription = true;
				break;
			}
			case "update":
				await updatePreferences(endpoint, desired);
				break;
			case "unsubscribe":
				await deleteSubscription(endpoint);
				await dropBrowserSubscription();
				hasSubscription = false;
				endpoint = "";
				break;
			case "none":
				break;
		}

		// Only now — the control must not claim a state the server does not hold.
		preferences = desired;
	} catch {
		errorText = "Could not save that change. Nothing was turned on.";
	} finally {
		busyKind = null;
	}
}
</script>

<svelte:head>
	<title>Notifications · Alt</title>
</svelte:head>

<section class="page">
	<header class="masthead">
		<p class="eyebrow">Settings</p>
		<h1>Notifications</h1>
		<p class="prose">
			Alt tells you when something you asked for has finished, and once each
			morning when the day's entrance is ready. Notifications never carry the
			content itself — opening one brings you here to read it.
		</p>
	</header>

	{#if errorText}
		<p class="notice error" data-testid="notification-settings-error" role="alert">
			{errorText}
		</p>
	{/if}

	{#if !supported}
		<p class="notice">
			This browser cannot receive push notifications.
		</p>
	{/if}

	{#if !installed}
		<aside class="notice" data-testid="install-instructions">
			<p class="eyebrow">Before notifications can arrive</p>
			<p class="prose">
				On iPhone and iPad, notifications are only delivered to an app on the
				Home Screen. Open the share menu and choose <em>Add to Home Screen</em>,
				then open Alt from the icon.
			</p>
			<p class="prose">
				An installed app keeps its own session, so you will be asked to sign in
				once more the first time you open it.
			</p>
		</aside>
	{/if}

	{#if loading}
		<p class="prose loading">Loading your settings</p>
	{:else}
		<ul class="kinds">
			{#each NOTIFICATION_KINDS as kind (kind)}
				<li class="kind">
					<div class="copy">
						<p class="kind-label">{KIND_LABELS[kind]}</p>
						<p class="prose">{KIND_DESCRIPTIONS[kind]}</p>
					</div>
					<Switch
						checked={preferences[kind]}
						busy={busyKind === kind}
						disabled={!supported || (busyKind !== null && busyKind !== kind)}
						label={KIND_LABELS[kind]}
						data-testid={testId(kind)}
						onchange={(next) => setKind(kind, next)}
					/>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.page {
		max-width: 42rem;
		margin: 0 auto;
		padding: 1.5rem 1rem calc(2rem + env(safe-area-inset-bottom, 0px));
	}

	.masthead {
		margin-bottom: 2rem;
	}

	.eyebrow {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		margin-bottom: 0.5rem;
	}

	h1 {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		margin-bottom: 0.75rem;
	}

	.prose {
		font-size: 0.95rem;
		line-height: 1.72;
		color: var(--alt-charcoal);
		max-width: 65ch;
	}

	.notice {
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		padding: 1rem;
		margin-bottom: 1.5rem;
	}

	.notice.error {
		border-color: var(--alt-error);
	}

	/* A single pulsing word, not a spinner. */
	.loading {
		font-style: italic;
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	.kinds {
		list-style: none;
		padding: 0;
		margin: 0;
		border-top: 1px solid var(--surface-border);
	}

	.kind {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 1rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.copy {
		/* Without this the text refuses to wrap and pushes the switch off-screen
		   on narrow Android viewports. */
		min-width: 0;
		flex: 1;
	}

	.kind-label {
		font-weight: 600;
		margin-bottom: 0.25rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.loading {
			animation: none;
			opacity: 1;
		}
	}
</style>
