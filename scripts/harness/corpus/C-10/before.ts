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
	// Deliberate 500ms observation window so any wrongly fired request reaches the route handler.
	await page.waitForTimeout(500);
	expect(wasApiCalled()).toBe(false);
}
