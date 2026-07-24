import { describe, expect, it } from "vitest";
import { MARK_AS_READ_FAILED_MESSAGE } from "./mark-as-read-feedback";

describe("mark-as-read-feedback", () => {
	it("exposes a stable user-facing error for failed MarkAsRead", () => {
		expect(MARK_AS_READ_FAILED_MESSAGE).toMatch(/still unread/i);
		expect(MARK_AS_READ_FAILED_MESSAGE.length).toBeGreaterThan(10);
	});
});
