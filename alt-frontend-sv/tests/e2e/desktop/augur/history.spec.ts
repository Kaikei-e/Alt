import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

const LIST_CONVERSATIONS_PATH =
	"**/api/v2/alt.augur.v2.AugurService/ListConversations";
const DELETE_CONVERSATION_PATH =
	"**/api/v2/alt.augur.v2.AugurService/DeleteConversation";
const GET_CONVERSATION_PATH =
	"**/api/v2/alt.augur.v2.AugurService/GetConversation";

const MOCK_CONVERSATIONS = [
	{
		id: "conv-101",
		title: "What changed in Rust 2024?",
		createdAt: "2026-03-20T10:00:00Z",
		lastActivityAt: "2026-03-20T10:05:00Z",
		lastMessagePreview: "Async fn in traits is now stabilized.",
		messageCount: 2,
	},
	{
		id: "conv-102",
		title: "Summarize today's AI papers",
		createdAt: "2026-03-19T08:00:00Z",
		lastActivityAt: "2026-03-19T08:10:00Z",
		lastMessagePreview: "Three papers focus on inference scaling.",
		messageCount: 4,
	},
];

const MOCK_CONVERSATION_DETAIL = {
	id: "conv-101",
	title: "What changed in Rust 2024?",
	createdAt: "2026-03-20T10:00:00Z",
	messages: [
		{
			role: "user",
			content: "What changed in Rust 2024?",
			createdAt: "2026-03-20T10:00:00Z",
			citations: [],
			relatedCitations: [],
		},
		{
			role: "assistant",
			content: "Async fn in traits is now stabilized in Rust 2024.",
			createdAt: "2026-03-20T10:05:00Z",
			citations: [
				{
					url: "https://example.com/rust-2024",
					title: "Rust 2024 Release Notes",
					publishedAt: "2026-03-20T00:00:00Z",
					kind: 1,
					refId: "",
				},
			],
			relatedCitations: [],
		},
	],
};

test.describe("Desktop Augur History & Conversation Detail", () => {
	test("lists conversations via ListConversations and deletes a conversation via DeleteConversation", async ({
		page,
	}) => {
		let deletedId: string | null = null;
		await page.route(LIST_CONVERSATIONS_PATH, (route) =>
			fulfillJson(route, {
				conversations: MOCK_CONVERSATIONS,
				nextPageToken: "",
			}),
		);
		await page.route(DELETE_CONVERSATION_PATH, async (route) => {
			const body = route.request().postDataJSON() as { id?: string };
			deletedId = body.id ?? null;
			await fulfillJson(route, {});
		});

		await page.goto("./augur/history");

		await expect(
			page.getByRole("heading", { name: "Conversations" }),
		).toBeVisible();
		await expect(page.getByText("What changed in Rust 2024?")).toBeVisible();
		await expect(page.getByText("Summarize today's AI papers")).toBeVisible();

		// Delete first conversation
		const deleteButtons = page.getByRole("button", {
			name: "Delete conversation",
		});
		await expect(deleteButtons).toHaveCount(2);
		await deleteButtons.first().click();

		await expect.poll(() => deletedId, { timeout: 10000 }).toBe("conv-101");
		await expect(
			page.getByText("What changed in Rust 2024?"),
		).not.toBeVisible();
		await expect(page.getByText("Summarize today's AI papers")).toBeVisible();
	});

	test("navigates from history to /augur/[id] and renders stored messages via GetConversation", async ({
		page,
	}) => {
		await page.route(LIST_CONVERSATIONS_PATH, (route) =>
			fulfillJson(route, {
				conversations: MOCK_CONVERSATIONS,
				nextPageToken: "",
			}),
		);
		await page.route(GET_CONVERSATION_PATH, (route) =>
			fulfillJson(route, MOCK_CONVERSATION_DETAIL),
		);

		await page.goto("./augur/history");

		await page.getByText("What changed in Rust 2024?").click();
		await expect(page).toHaveURL(/\/augur\/conv-101$/);

		await expect(
			page.getByText("Async fn in traits is now stabilized in Rust 2024."),
		).toBeVisible({ timeout: 10000 });
	});
});
