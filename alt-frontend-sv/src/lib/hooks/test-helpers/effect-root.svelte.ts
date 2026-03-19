/**
 * Creates a reactive boolean that can be toggled from tests.
 * Must be in .svelte.ts to use $state rune.
 */
export function createReactiveFlag(initial: boolean) {
	let value = $state(initial);
	return {
		get value() {
			return value;
		},
		set value(v: boolean) {
			value = v;
		},
	};
}

/**
 * Creates a reactive string state that can be changed from tests.
 */
export function createReactiveString(initial: string | undefined) {
	let value = $state<string | undefined>(initial);
	return {
		get value() {
			return value;
		},
		set value(v: string | undefined) {
			value = v;
		},
	};
}
