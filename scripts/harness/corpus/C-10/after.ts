interface PageLike {
	waitForTimeout(ms: number): Promise<void>;
	waitForLoadState(state: "load" | "domcontentloaded" | "networkidle"): Promise<void>;
}

declare function expect(actual: unknown): { toBe(expected: unknown): void };

export async function assertImeEnterDoesNotSend(
	page: PageLike,
	triggerImeEnter: () => Promise<void>,
	wasApiCalled: () => boolean,
): Promise<void> {
	await triggerImeEnter();
	// BUG: no-op wait on an already-loaded page eliminates the observation window.
	await page.waitForLoadState("domcontentloaded");
	expect(wasApiCalled()).toBe(false);
}
