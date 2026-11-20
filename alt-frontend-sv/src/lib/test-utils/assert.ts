/**
 * Runtime-checked narrowing helper for tests.
 * Replaces non-null assertions (`!`) so a missing value fails the test loudly
 * instead of passing silently on optional chaining (`?.`).
 */
export function assertDefined<T>(
	value: T | null | undefined,
	message = "Expected value to be defined",
): T {
	if (value === null || value === undefined) {
		throw new Error(message);
	}
	return value;
}
