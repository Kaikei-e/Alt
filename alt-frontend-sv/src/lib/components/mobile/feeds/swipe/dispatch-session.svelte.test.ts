/**
 * Dispatch session store tests.
 *
 * Undo semantics: the last swiped card can be returned to the pile. Both
 * `pass` (mark read) and `queue` (Read today) are held in a single-slot
 * pending buffer and committed by kind on the next dismissal or flush.
 */
import { describe, expect, it, vi } from "vitest";
import { createDispatchSession } from "./dispatch-session.svelte";

describe("createDispatchSession", () => {
	it("starts with an empty session", () => {
		const session = createDispatchSession({
			onPass: vi.fn(),
			onQueue: vi.fn(),
		});
		expect(session.readCount).toBe(0);
		expect(session.canUndo).toBe(false);
		expect(session.pending).toBeNull();
	});

	it("defers pass on dismiss until the next dismissal", () => {
		const onPass = vi.fn(() => Promise.resolve());
		const onQueue = vi.fn(() => Promise.resolve());
		const session = createDispatchSession({ onPass, onQueue });

		session.dismiss("https://a.example/", "pass");
		expect(session.readCount).toBe(1);
		expect(session.canUndo).toBe(true);
		expect(session.pending).toEqual({
			link: "https://a.example/",
			kind: "pass",
		});
		expect(onPass).not.toHaveBeenCalled();

		session.dismiss("https://b.example/", "pass");
		expect(session.readCount).toBe(2);
		expect(onPass).toHaveBeenCalledExactlyOnceWith("https://a.example/");
		expect(onQueue).not.toHaveBeenCalled();
	});

	it("defers queue on dismiss and flushes by kind", () => {
		const onPass = vi.fn(() => Promise.resolve());
		const onQueue = vi.fn(() => Promise.resolve());
		const session = createDispatchSession({ onPass, onQueue });

		session.dismiss("https://a.example/", "queue");
		expect(session.readCount).toBe(1);
		expect(session.canUndo).toBe(true);
		expect(onQueue).not.toHaveBeenCalled();

		session.dismiss("https://b.example/", "pass");
		expect(session.readCount).toBe(2);
		expect(onQueue).toHaveBeenCalledExactlyOnceWith("https://a.example/");
		expect(onPass).not.toHaveBeenCalled();

		session.flush();
		expect(onPass).toHaveBeenCalledExactlyOnceWith("https://b.example/");
	});

	it("undo returns the pending entry without committing either pass or queue", () => {
		const onPass = vi.fn(() => Promise.resolve());
		const onQueue = vi.fn(() => Promise.resolve());
		const session = createDispatchSession({ onPass, onQueue });

		session.dismiss("https://a.example/", "queue");
		const restored = session.undo();

		expect(restored).toEqual({
			link: "https://a.example/",
			kind: "queue",
		});
		expect(session.readCount).toBe(0);
		expect(session.canUndo).toBe(false);
		expect(session.pending).toBeNull();
		expect(onPass).not.toHaveBeenCalled();
		expect(onQueue).not.toHaveBeenCalled();
	});

	it("undo only reaches one card back", () => {
		const onPass = vi.fn(() => Promise.resolve());
		const onQueue = vi.fn(() => Promise.resolve());
		const session = createDispatchSession({ onPass, onQueue });

		session.dismiss("https://a.example/", "pass");
		session.dismiss("https://b.example/", "queue");
		const restored = session.undo();

		expect(restored).toEqual({
			link: "https://b.example/",
			kind: "queue",
		});
		expect(session.readCount).toBe(1);
		expect(session.undo()).toBeNull();
		expect(onPass).toHaveBeenCalledExactlyOnceWith("https://a.example/");
		expect(onQueue).not.toHaveBeenCalled();
	});

	it("notifies onPendingChange across dismiss, undo, and flush", () => {
		const onPendingChange = vi.fn();
		const session = createDispatchSession({
			onPass: vi.fn(() => Promise.resolve()),
			onQueue: vi.fn(() => Promise.resolve()),
			onPendingChange,
		});

		session.dismiss("https://a.example/", "queue");
		expect(onPendingChange).toHaveBeenLastCalledWith({
			link: "https://a.example/",
			kind: "queue",
		});

		session.undo();
		expect(onPendingChange).toHaveBeenLastCalledWith(null);

		session.dismiss("https://b.example/", "pass");
		session.flush();
		expect(onPendingChange).toHaveBeenLastCalledWith(null);
	});

	it("flush commits the pending entry and closes the undo window", () => {
		const onPass = vi.fn(() => Promise.resolve());
		const onQueue = vi.fn(() => Promise.resolve());
		const session = createDispatchSession({ onPass, onQueue });

		session.dismiss("https://a.example/", "queue");
		session.flush();

		expect(onQueue).toHaveBeenCalledExactlyOnceWith("https://a.example/");
		expect(session.canUndo).toBe(false);
		expect(session.readCount).toBe(1);

		session.flush();
		expect(onQueue).toHaveBeenCalledOnce();
	});

	it("swallows commit failures", () => {
		const onPass = vi.fn(() => Promise.reject(new Error("offline")));
		const onQueue = vi.fn(() => Promise.reject(new Error("offline")));
		const session = createDispatchSession({ onPass, onQueue });

		session.dismiss("https://a.example/", "pass");
		expect(() => session.flush()).not.toThrow();

		session.dismiss("https://b.example/", "queue");
		expect(() => session.flush()).not.toThrow();
	});
});
