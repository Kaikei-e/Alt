import { test as base, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { env } from "./env.js";

type TtsFixtures = {
	tts: APIRequestContext;
};

export const test = base.extend<Record<never, never>, TtsFixtures>({
	tts: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],
});

export { expect };
