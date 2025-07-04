import { describe, it, expect } from "vitest";
import { getStartOfLocalDayUTC } from "@/lib/utils/time";

describe("getStartOfLocalDayUTC", () => {
  it("should return UTC midnight for given local date", () => {
    const date = new Date("2024-05-27T15:30:00Z");
    const result = getStartOfLocalDayUTC(date);
    const expected = new Date(
      Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()),
    );
    expect(result.toISOString()).toBe(expected.toISOString());
  });
});
