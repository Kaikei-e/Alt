/**
 * Dispatch session store tests.
 *
 * Undo semantics (contexts/visual-preview.md): the last swiped card can be
 * returned to the pile. There is no un-read RPC, so the mark-as-read call
 * for a dismissed card is deferred until the next dismissal (or flush);
 * undo cancels the pending call instead of reverting the backend.
 */
import { describe, expect, it, vi } from "vitest";
import { createDispatchSession } from "./dispatch-session.svelte";

describe("createDispatchSession", () => {
	it("starts with an empty session", () => {
		const session = createDispatchSession(vi.fn());
		expect(session.readCount).toBe(0);
		expect(session.canUndo).toBe(false);
	});

	it("defers mark-as-read on dismiss until the next dismissal", () => {
		const markRead = vi.fn(() => Promise.resolve());
		const session = createDispatchSession(markRead);

		session.dismiss("https://a.example/");
		expect(session.readCount).toBe(1);
		expect(session.canUndo).toBe(true);
		expect(markRead).not.toHaveBeenCalled();

		session.dismiss("https://b.example/");
		expect(session.readCount).toBe(2);
		expect(markRead).toHaveBeenCalledExactlyOnceWith("https://a.example/");
	});

	it("undo returns the pending link without marking it read", () => {
		const markRead = vi.fn(() => Promise.resolve());
		const session = createDispatchSession(markRead);

		session.dismiss("https://a.example/");
		const restored = session.undo();

		expect(restored).toBe("https://a.example/");
		expect(session.readCount).toBe(0);
		expect(session.canUndo).toBe(false);
		expect(markRead).not.toHaveBeenCalled();
	});

	it("undo only reaches one card back", () => {
		const markRead = vi.fn(() => Promise.resolve());
		const session = createDispatchSession(markRead);

		session.dismiss("https://a.example/");
		session.dismiss("https://b.example/");
		const restored = session.undo();

		expect(restored).toBe("https://b.example/");
		expect(session.readCount).toBe(1);
		expect(session.undo()).toBeNull();
		expect(markRead).toHaveBeenCalledExactlyOnceWith("https://a.example/");
	});

	it("flush commits the pending read and closes the undo window", () => {
		const markRead = vi.fn(() => Promise.resolve());
		const session = createDispatchSession(markRead);

		session.dismiss("https://a.example/");
		session.flush();

		expect(markRead).toHaveBeenCalledExactlyOnceWith("https://a.example/");
		expect(session.canUndo).toBe(false);
		expect(session.readCount).toBe(1);

		session.flush();
		expect(markRead).toHaveBeenCalledOnce();
	});

	it("swallows mark-as-read failures", () => {
		const markRead = vi.fn(() => Promise.reject(new Error("offline")));
		const session = createDispatchSession(markRead);

		session.dismiss("https://a.example/");
		expect(() => session.flush()).not.toThrow();
	});
});
