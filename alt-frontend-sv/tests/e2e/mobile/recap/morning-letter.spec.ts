import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillConnectStream } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_MORNING_LETTER_STREAM_MESSAGES,
} from "../../fixtures/mockData";

test.describe("Mobile Morning Letter", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, (route) =>
			fulfillConnectStream(route, CONNECT_MORNING_LETTER_STREAM_MESSAGES),
		);
	});

	test("chat interface renders", async ({ mobileMorningLetterPage }) => {
		await mobileMorningLetterPage.goto();
		await mobileMorningLetterPage.waitForChatReady();
		await expect(mobileMorningLetterPage.chatInput).toBeVisible();
	});

	test("welcome message is visible", async ({ mobileMorningLetterPage }) => {
		await mobileMorningLetterPage.goto();
		await mobileMorningLetterPage.waitForChatReady();
		await expect(mobileMorningLetterPage.welcomeMessage).toBeVisible();
	});

	test("send message triggers streaming response", async ({
		page,
		mobileMorningLetterPage,
	}) => {
		await mobileMorningLetterPage.goto();
		await mobileMorningLetterPage.waitForChatReady();

		await mobileMorningLetterPage.sendMessage("What happened today?");

		// Should show thinking state or response
		await expect(
			mobileMorningLetterPage.thinkingIndicator
				.or(page.getByText(/past 24 hours/i))
				.first(),
		).toBeVisible({ timeout: 10000 });
	});

});
