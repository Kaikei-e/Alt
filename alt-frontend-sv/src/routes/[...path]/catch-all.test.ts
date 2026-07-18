import { describe, expect, it } from "vitest";
import { load } from "./+page.server";

describe("catch-all 404", () => {
	it("should throw a 404 HttpError for unmatched paths", async () => {
		try {
			await load({ url: new URL("http://localhost/nonexistent") } as never);
			expect.unreachable("expected load to throw");
		} catch (e) {
			expect(e).toMatchObject({ status: 404 });
		}
	});

	it("should throw a 404 HttpError for deeply nested unmatched paths", async () => {
		try {
			await load({ url: new URL("http://localhost/a/b/c/d") } as never);
			expect.unreachable("expected load to throw");
		} catch (e) {
			expect(e).toMatchObject({ status: 404 });
		}
	});
});
