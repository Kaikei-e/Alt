import { describe, it, expect } from "vitest";
import { uuidv7 } from "./uuidv7";

const UUID_RE =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

describe("uuidv7", () => {
	it("produces a canonical 36-char hyphenated UUID", () => {
		const id = uuidv7();
		expect(id).toHaveLength(36);
		expect(id).toMatch(UUID_RE);
	});

	it("has version nibble = 7 at position 14 (UUIDv7 spec)", () => {
		for (let i = 0; i < 20; i++) {
			const id = uuidv7();
			expect(id[14]).toBe("7");
		}
	});

	it("has variant nibble 8/9/a/b at position 19 (RFC 4122 variant bits)", () => {
		for (let i = 0; i < 20; i++) {
			const id = uuidv7();
			expect(["8", "9", "a", "b"]).toContain(id[19].toLowerCase());
		}
	});

	it("encodes the unix-ms timestamp in the first 48 bits", () => {
		const before = Date.now();
		const id = uuidv7();
		const after = Date.now();

		// First 48 bits = first 12 hex chars (8+4 across the first two groups).
		const hex = id.replace(/-/g, "").slice(0, 12);
		const ts = Number.parseInt(hex, 16);

		expect(ts).toBeGreaterThanOrEqual(before);
		expect(ts).toBeLessThanOrEqual(after);
	});

	it("returns monotonically non-decreasing values under rapid-fire calls", () => {
		const ids = Array.from({ length: 50 }, () => uuidv7());
		for (let i = 1; i < ids.length; i++) {
			expect(ids[i] >= ids[i - 1]).toBe(true);
		}
	});

	it("produces unique values (no collisions across 1000 calls)", () => {
		const seen = new Set<string>();
		for (let i = 0; i < 1000; i++) seen.add(uuidv7());
		expect(seen.size).toBe(1000);
	});
});
