import { describe, it, expect } from "vitest";
import { resolveAutostartIntent } from "./acolyteAutostartParams";

describe("resolveAutostartIntent", () => {
	it("returns kind='resume' with the runId when ?run= is present", () => {
		const params = new URLSearchParams("run=run-001");
		expect(resolveAutostartIntent(params)).toEqual({
			kind: "resume",
			runId: "run-001",
		});
	});

	it("returns kind='autostart-failed' when ?autostart_failed=1 is present and ?run is absent", () => {
		const params = new URLSearchParams("autostart_failed=1");
		expect(resolveAutostartIntent(params)).toEqual({
			kind: "autostart-failed",
		});
	});

	it("returns kind='none' when neither query param is present", () => {
		expect(resolveAutostartIntent(new URLSearchParams(""))).toEqual({
			kind: "none",
		});
	});

	it("ignores empty ?run= value", () => {
		expect(resolveAutostartIntent(new URLSearchParams("run="))).toEqual({
			kind: "none",
		});
	});

	it("prefers ?run= over ?autostart_failed when both are present", () => {
		const params = new URLSearchParams("run=run-002&autostart_failed=1");
		expect(resolveAutostartIntent(params)).toEqual({
			kind: "resume",
			runId: "run-002",
		});
	});

	it("treats autostart_failed values other than '1' as no-op", () => {
		expect(
			resolveAutostartIntent(new URLSearchParams("autostart_failed=0")),
		).toEqual({ kind: "none" });
	});
});
