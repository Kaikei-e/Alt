import { expect, test } from "@playwright/test";
import { DesktopAugurPage } from "../../pages/desktop/DesktopAugurPage";
import { fulfillStream, fulfillConnectStream } from "../../utils/mockHelpers";
import {
	AUGUR_RESPONSE_CHUNKS,
	CONNECT_AUGUR_STREAM_MESSAGES,
	CONNECT_AUGUR_SIMPLE_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("Desktop Augur Chat", () => {
	let augurPage: DesktopAugurPage;

	test.beforeEach(async ({ page }) => {
		augurPage = new DesktopAugurPage(page);
	});

	test("renders page title and welcome message", async () => {
		await augurPage.goto();

		// Verify page title
		await expect(augurPage.pageTitle).toBeVisible();
		await expect(augurPage.pageTitle).toContainText("Ask Augur");

		// Verify welcome message
		await augurPage.waitForWelcomeMessage();
	});

	test("chat interface has input and send button", async () => {
		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		// Verify input elements
		await expect(augurPage.chatInput).toBeVisible();
		await expect(augurPage.sendButton).toBeVisible();
	});

	test("can type in chat input", async () => {
		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		const testMessage = "What are the latest AI trends?";
		await augurPage.chatInput.fill(testMessage);

		await expect(augurPage.chatInput).toHaveValue(testMessage);
	});

	test("sends user message and displays it", async ({ page }) => {
		// Mock Connect-RPC streaming response
		await page.route(CONNECT_RPC_PATHS.augurStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_AUGUR_STREAM_MESSAGES);
		});

		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		const userMessage = "What are the latest AI trends?";
		await augurPage.sendMessage(userMessage);

		// Verify user message appears in chat
		await expect(page.getByText(userMessage)).toBeVisible();
	});

	test("shows thinking indicator while waiting for response", async ({ page }) => {
		// Mock with delay to observe loading state
		await page.route(CONNECT_RPC_PATHS.augurStreamChat, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillConnectStream(route, CONNECT_AUGUR_STREAM_MESSAGES);
		});

		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		await augurPage.sendMessage("Test question");

		// Thinking indicator should appear
		await expect(augurPage.thinkingIndicator).toBeVisible();
	});

	test("displays AI response after sending message", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.augurStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_AUGUR_STREAM_MESSAGES);
		});

		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		await augurPage.sendMessage("What are the latest AI trends?");

		// Wait for response
		await augurPage.waitForResponse();

		// Verify response appears (content from CONNECT_AUGUR_STREAM_MESSAGES)
		await expect(
			page.getByText(/based on your recent feeds/i),
		).toBeVisible({ timeout: 10000 });
	});

	test("disables input while processing", async ({ page }) => {
		// Mock with delay
		await page.route(CONNECT_RPC_PATHS.augurStreamChat, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await fulfillConnectStream(route, CONNECT_AUGUR_STREAM_MESSAGES);
		});

		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		await augurPage.sendMessage("Test");

		// Input should be disabled while loading
		const isDisabled = await augurPage.isInputDisabled();
		expect(isDisabled).toBe(true);
	});

	test("handles error gracefully", async ({ page }) => {
		// Mock error response
		await page.route(CONNECT_RPC_PATHS.augurStreamChat, async (route) => {
			await route.fulfill({
				status: 500,
				contentType: "application/json",
				body: JSON.stringify({ code: "internal", message: "Server error" }),
			});
		});

		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		await augurPage.sendMessage("Test question");

		// Should show error message
		await expect(page.getByText(/error/i)).toBeVisible({ timeout: 10000 });
	});
});

test.describe("Desktop Augur Chat - Conversation", () => {
	test("maintains conversation history", async ({ page }) => {
		const augurPage = new DesktopAugurPage(page);

		await page.route(CONNECT_RPC_PATHS.augurStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_AUGUR_SIMPLE_RESPONSE);
		});

		await augurPage.goto();
		await augurPage.waitForWelcomeMessage();

		// Send first message
		await augurPage.sendMessage("First question");
		await augurPage.waitForResponse();

		// Verify both user question and response are visible
		await expect(page.getByText("First question")).toBeVisible();
		await expect(page.getByText("Response to your question")).toBeVisible();

		// Send second message
		await augurPage.sendMessage("Second question");
		await augurPage.waitForResponse();

		// All messages should still be visible
		await expect(page.getByText("First question")).toBeVisible();
		await expect(page.getByText("Second question")).toBeVisible();
	});
});
