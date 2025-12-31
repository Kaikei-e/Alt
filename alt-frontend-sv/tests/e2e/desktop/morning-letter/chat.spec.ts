import { expect, test } from "@playwright/test";
import { DesktopMorningLetterPage } from "../../pages/desktop/DesktopMorningLetterPage";
import { fulfillConnectStream } from "../../utils/mockHelpers";
import {
	CONNECT_MORNING_LETTER_STREAM_MESSAGES,
	CONNECT_MORNING_LETTER_SIMPLE_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("Desktop Morning Letter Chat", () => {
	let morningLetterPage: DesktopMorningLetterPage;

	test.beforeEach(async ({ page }) => {
		morningLetterPage = new DesktopMorningLetterPage(page);
	});

	test("renders page title and welcome message", async () => {
		await morningLetterPage.goto();

		// Verify page title
		await expect(morningLetterPage.pageTitle).toBeVisible();
		await expect(morningLetterPage.pageTitle).toContainText("Morning Letter");

		// Verify welcome message
		await morningLetterPage.waitForWelcomeMessage();
	});

	test("chat interface has input and send button", async () => {
		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		// Verify input elements
		await expect(morningLetterPage.chatInput).toBeVisible();
		await expect(morningLetterPage.sendButton).toBeVisible();
	});

	test("can type in chat input", async () => {
		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		const testMessage = "What happened in tech today?";
		await morningLetterPage.chatInput.fill(testMessage);

		await expect(morningLetterPage.chatInput).toHaveValue(testMessage);
	});

	test("sends user message and displays it", async ({ page }) => {
		// Mock Connect-RPC streaming response
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_MORNING_LETTER_STREAM_MESSAGES);
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		const userMessage = "What are the top stories today?";
		await morningLetterPage.sendMessage(userMessage);

		// Verify user message appears in chat
		await expect(page.getByText(userMessage)).toBeVisible();
	});

	test("shows thinking indicator while waiting for response", async ({ page }) => {
		// Mock with longer delay to ensure loading state is observable
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 2000));
			await fulfillConnectStream(route, CONNECT_MORNING_LETTER_STREAM_MESSAGES);
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		await morningLetterPage.sendMessage("Test question");

		// Thinking indicator should appear (allow some time for state update)
		await expect(morningLetterPage.thinkingIndicator).toBeVisible({ timeout: 3000 });
	});

	test("displays AI response after sending message", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_MORNING_LETTER_STREAM_MESSAGES);
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		await morningLetterPage.sendMessage("What's in the news today?");

		// Wait for response
		await morningLetterPage.waitForResponse();

		// Verify response appears (content from CONNECT_MORNING_LETTER_STREAM_MESSAGES)
		await expect(
			page.getByText(/based on the past 24 hours/i),
		).toBeVisible({ timeout: 10000 });
	});

	test("displays citations in response", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_MORNING_LETTER_STREAM_MESSAGES);
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		await morningLetterPage.sendMessage("Tell me about AI news");

		// Wait for response
		await morningLetterPage.waitForResponse();

		// Verify citations appear
		await expect(page.getByText(/AI Research Update/i)).toBeVisible({ timeout: 10000 });
		await expect(page.getByText(/Tech Weekly/i)).toBeVisible({ timeout: 10000 });
	});

	test("disables input while processing", async ({ page }) => {
		// Mock with delay
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await fulfillConnectStream(route, CONNECT_MORNING_LETTER_STREAM_MESSAGES);
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		await morningLetterPage.sendMessage("Test");

		// Input should be disabled while loading
		const isDisabled = await morningLetterPage.isInputDisabled();
		expect(isDisabled).toBe(true);
	});

	test("handles error gracefully", async ({ page }) => {
		// Mock error response
		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await route.fulfill({
				status: 500,
				contentType: "application/json",
				body: JSON.stringify({ code: "internal", message: "Server error" }),
			});
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		await morningLetterPage.sendMessage("Test question");

		// Should show error message
		await expect(page.getByText(/error/i)).toBeVisible({ timeout: 10000 });
	});
});

test.describe("Desktop Morning Letter Chat - Conversation", () => {
	test("maintains conversation history", async ({ page }) => {
		const morningLetterPage = new DesktopMorningLetterPage(page);

		await page.route(CONNECT_RPC_PATHS.morningLetterStreamChat, async (route) => {
			await fulfillConnectStream(route, CONNECT_MORNING_LETTER_SIMPLE_RESPONSE);
		});

		await morningLetterPage.goto();
		await morningLetterPage.waitForWelcomeMessage();

		// Send first message
		await morningLetterPage.sendMessage("First question");
		await morningLetterPage.waitForResponse();

		// Verify both user question and response are visible
		await expect(page.getByText("First question")).toBeVisible();
		await expect(page.getByText("Here is your morning briefing")).toBeVisible();

		// Send second message
		await morningLetterPage.sendMessage("Second question");
		await morningLetterPage.waitForResponse();

		// All messages should still be visible
		await expect(page.getByText("First question")).toBeVisible();
		await expect(page.getByText("Second question")).toBeVisible();
	});
});
