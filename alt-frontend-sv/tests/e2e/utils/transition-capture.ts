import type { Page, Route } from "@playwright/test";

/**
 * Captured POST body shape for /loop/transition. Field names follow the
 * BFF contract (camelCase) but legacy snake_case keys are also tolerated
 * because earlier specs assert against either form (see
 * loop-tap-advances-orient.spec.ts).
 */
export type CapturedTransitionPost = Record<string, unknown>;

export interface TransitionCapture {
	posts: CapturedTransitionPost[];
	/**
	 * Resolves the next BFF reply. Useful when the test wants to prove
	 * optimistic UI behavior before letting the server reply land. Defaults to
	 * an immediate `accepted=true` reply if not held.
	 */
	release: () => void;
}

const TRANSITION_PATH = "**/loop/transition";

export interface InstallTransitionCaptureOptions {
	/** If true, hold the BFF reply until `capture.release()` is called. */
	hold?: boolean;
	/** Override the canonical entry key in the success reply. */
	canonicalEntryKey?: string;
	/** Override the success accepted flag. */
	accepted?: boolean;
}

/**
 * Intercept POST /loop/transition, capture each request body as parsed JSON,
 * and reply with a success envelope. Use the returned object's `posts` array
 * after `expect.poll(() => capture.posts.length)` to assert request shapes.
 *
 * Reference: Playwright Mock APIs (https://playwright.dev/docs/mock) and
 *            class Route (https://playwright.dev/docs/api/class-route).
 */
export async function installTransitionCapture(
	page: Page,
	opts: InstallTransitionCaptureOptions = {},
): Promise<TransitionCapture> {
	const posts: CapturedTransitionPost[] = [];

	let release: () => void = () => {};
	const released = opts.hold
		? new Promise<void>((resolve) => {
				release = resolve;
			})
		: Promise.resolve();

	await page.route(TRANSITION_PATH, async (route: Route) => {
		if (route.request().method() === "POST") {
			try {
				posts.push(route.request().postDataJSON() as CapturedTransitionPost);
			} catch {
				/* ignore; capture remains best-effort */
			}
		}
		await released;
		await route.fulfill({
			status: 200,
			contentType: "application/json",
			body: JSON.stringify({
				accepted: opts.accepted ?? true,
				canonicalEntryKey: opts.canonicalEntryKey ?? "",
			}),
		});
	});

	return { posts, release };
}

/**
 * Pluck the camelCase field, falling back to snake_case for compatibility
 * with the BFF's legacy spread shape. The BFF flattens `metadata.*` into the
 * top-level body, so semantic fields appear at the root.
 */
export function field<T = unknown>(
	post: CapturedTransitionPost,
	camel: string,
	snake: string,
): T | undefined {
	return (post[camel] ?? post[snake]) as T | undefined;
}
