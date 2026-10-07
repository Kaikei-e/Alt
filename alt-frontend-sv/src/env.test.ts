import { beforeEach, describe, expect, it, vi } from "vitest";
import { variables } from "./env";

const { environment } = vi.hoisted(() => ({
	environment: { building: false, dev: false },
}));

vi.mock("$app/env", () => ({
	get building() {
		return environment.building;
	},
	get dev() {
		return environment.dev;
	},
	browser: false,
	version: "test",
}));

function assertIssues(result: unknown) {
	if (result && typeof result === "object" && "issues" in result) {
		return (result as { issues: Array<{ message: string }> }).issues;
	}
	throw new Error(
		`Expected result to have issues, got: ${JSON.stringify(result)}`,
	);
}

describe("src/env.ts schema validation", () => {
	beforeEach(() => {
		environment.building = false;
		environment.dev = false;
	});

	describe("KRATOS_PUBLIC_URL", () => {
		const schema = variables.KRATOS_PUBLIC_URL.schema["~standard"];

		it("accepts valid absolute URLs", () => {
			const result = schema.validate("http://kratos.local:4433");
			expect(result).toEqual({ value: "http://kratos.local:4433" });

			const httpsResult = schema.validate("https://auth.example.com/ory");
			expect(httpsResult).toEqual({ value: "https://auth.example.com/ory" });
		});

		it("rejects relative URLs with a descriptive error issue", () => {
			const result = schema.validate("/relative/path");
			const issues = assertIssues(result);
			expect(issues[0]?.message).toMatch(/absolute URL/);
		});

		it("returns empty string when building and variable is unset", () => {
			environment.building = true;
			const result = schema.validate(undefined);
			expect(result).toEqual({ value: "" });
		});

		it("returns dev fallback when dev is true and variable is unset", () => {
			environment.dev = true;
			const result = schema.validate(undefined);
			expect(result).toEqual({ value: "http://localhost/ory" });
		});

		it("fails with a clear error issue in production when variable is unset", () => {
			const result = schema.validate(undefined);
			const issues = assertIssues(result);
			expect(issues[0]?.message).toMatch(/KRATOS_PUBLIC_URL must be set/);
		});
	});

	describe("ORIGIN", () => {
		const schema = variables.ORIGIN.schema["~standard"];

		it("accepts valid origin values", () => {
			const result = schema.validate("https://alt.example.com");
			expect(result).toEqual({ value: "https://alt.example.com" });
		});

		it("returns empty string when building and variable is unset", () => {
			environment.building = true;
			const result = schema.validate(undefined);
			expect(result).toEqual({ value: "" });
		});

		it("returns dev fallback when dev is true and variable is unset", () => {
			environment.dev = true;
			const result = schema.validate(undefined);
			expect(result).toEqual({ value: "http://localhost:4173" });
		});

		it("fails with a clear error issue in production when variable is unset", () => {
			const result = schema.validate(undefined);
			const issues = assertIssues(result);
			expect(issues[0]?.message).toMatch(/ORIGIN must be set/);
		});
	});

	describe("other variables", () => {
		it("retains input ?? '' fallback for optional variables", () => {
			const schema = variables.BACKEND_CONNECT_URL.schema["~standard"];
			expect(schema.validate(undefined)).toEqual({ value: "" });
			expect(schema.validate("http://backend:9000")).toEqual({
				value: "http://backend:9000",
			});
		});
	});
});
