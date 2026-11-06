import { describe, expect, it } from "vitest";

import { formatLocalTime, formatValue, localTimeZone } from "./format";

describe("formatValue", () => {
	it("returns em dash for null and non-finite values", () => {
		expect(formatValue(null, "seconds")).toBe("—");
		expect(formatValue(Number.NaN, "seconds")).toBe("—");
		expect(formatValue(Number.POSITIVE_INFINITY, "ratio")).toBe("—");
	});

	it("renders seconds as ms below 1s and as s above", () => {
		expect(formatValue(0.123, "seconds")).toBe("123 ms");
		expect(formatValue(1.5, "seconds")).toBe("1.50 s");
	});

	it("renders ratio as percent with two decimals", () => {
		expect(formatValue(0, "ratio")).toBe("0.00%");
		expect(formatValue(0.0142, "ratio")).toBe("1.42%");
	});

	it("renders bool as up/down", () => {
		expect(formatValue(1, "bool")).toBe("up");
		expect(formatValue(0, "bool")).toBe("down");
	});

	it("renders bytes in binary prefix", () => {
		expect(formatValue(1024, "bytes")).toBe("1.00 KiB");
		expect(formatValue(5 * 1024 * 1024, "bytes")).toBe("5.00 MiB");
	});
});

describe("formatLocalTime and localTimeZone", () => {
	it("returns a non-empty IANA time zone string", () => {
		expect(localTimeZone().length).toBeGreaterThan(0);
	});

	it("formats ISO strings, Date instances, and millisecond timestamps in local HH:MM:SS without seconds-guessing", () => {
		const iso = "2026-11-06T12:34:56Z";
		const d = new Date(iso);
		const expected = [
			String(d.getHours()).padStart(2, "0"),
			String(d.getMinutes()).padStart(2, "0"),
			String(d.getSeconds()).padStart(2, "0"),
		].join(":");

		expect(formatLocalTime(iso)).toBe(expected);
		expect(formatLocalTime(d.getTime())).toBe(expected);
		expect(formatLocalTime(d)).toBe(expected);
		expect(formatLocalTime(1_000)).toBe(
			[
				String(new Date(1_000).getHours()).padStart(2, "0"),
				String(new Date(1_000).getMinutes()).padStart(2, "0"),
				String(new Date(1_000).getSeconds()).padStart(2, "0"),
			].join(":"),
		);
	});
});
