import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { AugurConversationSummary } from "#lib/connect/index.js";
import ConversationList from "./ConversationList.svelte";

const sampleConversations: AugurConversationSummary[] = [
	{
		id: "conv-1",
		title: "Single Turn Inquiry",
		createdAt: new Date("2026-03-20T08:00:00Z"),
		lastActivityAt: new Date("2026-03-20T08:05:00Z"),
		lastMessagePreview: "One question asked",
		messageCount: 1,
	},
	{
		id: "conv-2",
		title: "Multi Turn Deep Dive",
		createdAt: new Date("2026-03-21T10:00:00Z"),
		lastActivityAt: new Date("2026-03-21T10:30:00Z"),
		lastMessagePreview: "Follow up answer",
		messageCount: 4,
	},
];

describe("ConversationList", () => {
	it("renders empty state when conversations list is empty and not loading", async () => {
		render(ConversationList, {
			props: {
				conversations: [],
				isLoading: false,
				errorMessage: "",
				hasMore: false,
				onOpen: vi.fn(),
				onDelete: vi.fn(),
				onLoadMore: vi.fn(),
				onStartNew: vi.fn(),
			},
		});

		await expect
			.element(page.getByText(/No conversations yet/i))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: "Ask Augur" }))
			.toBeInTheDocument();
	});

	it("renders conversations with proper singular/plural turn counts", async () => {
		render(ConversationList, {
			props: {
				conversations: sampleConversations,
				isLoading: false,
				errorMessage: "",
				hasMore: false,
				onOpen: vi.fn(),
				onDelete: vi.fn(),
				onLoadMore: vi.fn(),
				onStartNew: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Single Turn Inquiry"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("One question asked"))
			.toBeInTheDocument();
		await expect.element(page.getByText(/1\s+turn\b/)).toBeInTheDocument();

		await expect
			.element(page.getByText("Multi Turn Deep Dive"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Follow up answer"))
			.toBeInTheDocument();
		await expect.element(page.getByText(/4\s+turns\b/)).toBeInTheDocument();
	});

	it("calls onOpen(id) when a conversation row is clicked", async () => {
		const onOpen = vi.fn();
		const onDelete = vi.fn();

		render(ConversationList, {
			props: {
				conversations: sampleConversations,
				isLoading: false,
				errorMessage: "",
				hasMore: false,
				onOpen,
				onDelete,
				onLoadMore: vi.fn(),
				onStartNew: vi.fn(),
			},
		});

		await page.getByText("Multi Turn Deep Dive").click();

		expect(onOpen).toHaveBeenCalledWith("conv-2");
		expect(onDelete).not.toHaveBeenCalled();
	});

	it("calls onDelete(id) with stopPropagation when delete button is clicked without triggering onOpen", async () => {
		const onOpen = vi.fn();
		const onDelete = vi.fn();

		render(ConversationList, {
			props: {
				conversations: sampleConversations.slice(0, 1),
				isLoading: false,
				errorMessage: "",
				hasMore: false,
				onOpen,
				onDelete,
				onLoadMore: vi.fn(),
				onStartNew: vi.fn(),
			},
		});

		const deleteBtn = page.getByRole("button", {
			name: "Delete conversation",
		});
		await deleteBtn.click();

		expect(onDelete).toHaveBeenCalledWith("conv-1");
		expect(onOpen).not.toHaveBeenCalled();
	});
});
