import { expect } from "@playwright/test";
import type { APIResponse } from "@playwright/test";
import type { z } from "zod";

/**
 * Assertion helpers.
 *
 * The single most useful thing these add over raw `expect(response.status())`
 * is the response body in the failure message. A bare status assertion that
 * reports "expected 200, received 500" sends you to `docker compose logs`;
 * the same assertion carrying `{"error":{"code":"FEED_NOT_FOUND"}}` usually
 * ends the investigation on the spot.
 */

const BODY_PREVIEW_LIMIT = 2_000;

async function preview(response: APIResponse): Promise<string> {
	let text: string;
	try {
		text = await response.text();
	} catch {
		return "<body unavailable>";
	}
	return text.length > BODY_PREVIEW_LIMIT
		? `${text.slice(0, BODY_PREVIEW_LIMIT)}… (${text.length} bytes)`
		: text;
}

async function describe(response: APIResponse): Promise<string> {
	return `${response.url()} -> ${response.status()} ${response.statusText()}\n${await preview(response)}`;
}

/** Asserts an exact status, reporting the body when it does not match. */
export async function expectStatus(response: APIResponse, expected: number): Promise<void> {
	if (response.status() !== expected) {
		throw new Error(`expected status ${expected}\n${await describe(response)}`);
	}
}

/**
 * Asserts the status is one of `expected`.
 *
 * Used only where the Hurl suite already documented a legitimately bounded
 * outcome — e.g. a path whose success depends on whether the deps-stub has
 * grown a route yet. A band is a statement that *both* answers are correct,
 * never a shrug: every use of it in this suite carries a comment naming why
 * each member is in the set and why everything outside it is a regression.
 */
export async function expectStatusIn(
	response: APIResponse,
	expected: readonly number[],
): Promise<void> {
	if (!expected.includes(response.status())) {
		throw new Error(
			`expected status in [${expected.join(", ")}]\n${await describe(response)}`,
		);
	}
}

/** Parses the body against a schema, reporting the body when it does not fit. */
export async function expectJson<T>(
	response: APIResponse,
	schema: z.ZodType<T>,
): Promise<T> {
	const text = await response.text();

	let parsed: unknown;
	try {
		parsed = JSON.parse(text);
	} catch {
		throw new Error(
			`expected a JSON body\n${response.url()} -> ${response.status()}\n${text.slice(0, BODY_PREVIEW_LIMIT)}`,
		);
	}

	const result = schema.safeParse(parsed);
	if (!result.success) {
		throw new Error(
			`response body did not match its schema\n` +
				`${response.url()} -> ${response.status()}\n` +
				`${result.error.issues.map((issue) => `  - ${issue.path.join(".") || "<root>"}: ${issue.message}`).join("\n")}\n` +
				`body: ${text.slice(0, BODY_PREVIEW_LIMIT)}`,
		);
	}
	return result.data;
}

/** `expectStatus` + `expectJson` in the order you always want them. */
export async function expectJsonStatus<T>(
	response: APIResponse,
	expected: number,
	schema: z.ZodType<T>,
): Promise<T> {
	await expectStatus(response, expected);
	return expectJson(response, schema);
}

/**
 * Asserts a response header contains a substring, case-insensitively on the
 * header name (Playwright lower-cases them) and case-sensitively on the value.
 */
export function expectHeaderContains(
	response: APIResponse,
	name: string,
	needle: string,
): void {
	const value = response.headers()[name.toLowerCase()];
	expect(
		value,
		`header "${name}" of ${response.url()} (headers: ${JSON.stringify(response.headers())})`,
	).toContain(needle);
}

/** Asserts a response header is exactly `value`. */
export function expectHeader(response: APIResponse, name: string, value: string): void {
	const actual = response.headers()[name.toLowerCase()];
	expect(actual, `header "${name}" of ${response.url()}`).toBe(value);
}
