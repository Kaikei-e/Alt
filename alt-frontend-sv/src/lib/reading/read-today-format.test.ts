import { describe, expect, it } from "vitest";
import { formatTimeLeft } from "./read-today-format.js";

describe("formatTimeLeft", () => {
	it("formats whole hours remaining", () => {
		const now = new Date("2026-11-18T12:00:00.000Z");
		const expires = new Date("2026-11-20T05:30:00.000Z");
		expect(formatTimeLeft(expires, now)).toBe("41h left");
	});

	it("returns 'under 1h left' when less than 1 hour remains", () => {
		const now = new Date("2026-11-18T12:00:00.000Z");
		const expires = new Date("2026-11-18T12:45:00.000Z");
		expect(formatTimeLeft(expires, now)).toBe("under 1h left");
	});

	it("returns 'under 1h left' at or past expiry", () => {
		const now = new Date("2026-11-18T12:00:00.000Z");
		const expires = new Date("2026-11-18T11:50:00.000Z");
		expect(formatTimeLeft(expires, now)).toBe("under 1h left");
	});

	it("returns empty string when expiresAt is null", () => {
		expect(formatTimeLeft(null)).toBe("");
	});
});
