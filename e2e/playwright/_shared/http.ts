import { expect } from "@playwright/test";
import type { APIResponse } from "@playwright/test";
import type { z } from "zod";

/**
 * Assertion helpers shared by every Alt Playwright suite.
 *
 * The single most useful thing these add over raw `expect(res.status())` is
 * the response body in the failure message. A bare status assertion that
 * reports "expected 200, received 500" sends you to `docker compose logs`;
 * the same assertion carrying `{"error":{"code":"FEED_NOT_FOUND"}}` usually
 * ends the investigation on the spot.
 *
 * They are plain `async` functions rather than custom matchers on purpose:
 * a matcher would have to swallow the body read to stay synchronous, and the
 * body is the whole point.
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
 * A band is a statement that *both* answers are correct, never a shrug.
 * Every use of it must carry a comment naming why each member is in the set
 * and why everything outside it is a regression — otherwise it is a test that
 * cannot fail, which is worse than no test because it reports green.
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
			`expected a JSON body\n${response.url()} -> ${response.status()}\n` +
				`${text.slice(0, BODY_PREVIEW_LIMIT)}`,
		);
	}

	const result = schema.safeParse(parsed);
	if (!result.success) {
		throw new Error(
			`response body did not match its schema\n` +
				`${response.url()} -> ${response.status()}\n` +
				`${result.error.issues
					.map((issue) => `  - ${issue.path.join(".") || "<root>"}: ${issue.message}`)
					.join("\n")}\n` +
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

/**
 * Asserts a response header is absent.
 *
 * The negative direction matters as much as the positive one for the headers
 * that leak: `Server`, `X-Powered-By`, and any listener that should not be
 * answering at all.
 */
export function expectNoHeader(response: APIResponse, name: string): void {
	const actual = response.headers()[name.toLowerCase()];
	expect(
		actual,
		`header "${name}" should not be present on ${response.url()}`,
	).toBeUndefined();
}

/**
 * Asserts the body is Prometheus exposition text carrying at least one of the
 * named metric families.
 *
 * A `/metrics` endpoint that answers 200 with an empty body is the classic
 * silent-observability failure: the scrape succeeds, the dashboards stay
 * blank, and nothing alerts. Naming the families turns "it answered" into "it
 * is publishing what this service is supposed to publish".
 */
export async function expectPrometheusText(
	response: APIResponse,
	families: readonly string[] = [],
): Promise<string> {
	await expectStatus(response, 200);
	expectHeaderContains(response, "Content-Type", "text/plain");
	const body = await response.text();
	expect(body, `${response.url()} served no HELP lines`).toContain("# HELP");
	for (const family of families) {
		expect(body, `${response.url()} is not publishing metric family "${family}"`).toContain(
			family,
		);
	}
	return body;
}
