import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import {
	createTtsPreferences,
	TTS_PREFERENCES_KEY,
	TTS_PREFERENCES_STORAGE_KEY,
	type TtsPreferencesStore,
} from "./ttsPreferences.svelte";

function installLocalStorageShim(): Storage {
	const data = new Map<string, string>();
	const shim: Storage = {
		get length() {
			return data.size;
		},
		clear() {
			data.clear();
		},
		getItem(key) {
			return data.has(key) ? (data.get(key) as string) : null;
		},
		key(index) {
			return Array.from(data.keys())[index] ?? null;
		},
		removeItem(key) {
			data.delete(key);
		},
		setItem(key, value) {
			data.set(key, String(value));
		},
	};
	return shim;
}

describe("TtsPreferences", () => {
	let prefs: TtsPreferencesStore;

	beforeAll(() => {
		if (!globalThis.localStorage) {
			Object.defineProperty(globalThis, "localStorage", {
				value: installLocalStorageShim(),
				configurable: true,
				writable: true,
			});
		}
	});

	beforeEach(() => {
		// Each test starts from a clean localStorage so seed behaviour is
		// deterministic across the suite.
		globalThis.localStorage?.clear();
		prefs = createTtsPreferences();
	});

	describe("defaults", () => {
		it("defaults speed to 1.0", () => {
			expect(prefs.speed).toBe(1.0);
		});

		it("exposes the canonical speed choices", () => {
			expect(prefs.speedChoices).toEqual([0.75, 1.0, 1.25, 1.5, 2.0]);
		});
	});

	describe("setSpeed", () => {
		it("updates the in-memory speed value", () => {
			prefs.setSpeed(1.5);
			expect(prefs.speed).toBe(1.5);
		});

		it("persists the new speed to localStorage", () => {
			prefs.setSpeed(1.25);
			const raw = globalThis.localStorage?.getItem(TTS_PREFERENCES_STORAGE_KEY);
			expect(raw).not.toBeNull();
			expect(JSON.parse(raw as string)).toMatchObject({ speed: 1.25 });
		});

		it("rejects values outside the canonical speed choices", () => {
			expect(() => prefs.setSpeed(3.0)).toThrow();
			// state must stay at the prior valid value
			expect(prefs.speed).toBe(1.0);
		});
	});

	describe("hydration", () => {
		it("loads a previously persisted speed from localStorage", () => {
			globalThis.localStorage?.setItem(
				TTS_PREFERENCES_STORAGE_KEY,
				JSON.stringify({ speed: 1.5 }),
			);
			const hydrated = createTtsPreferences();
			expect(hydrated.speed).toBe(1.5);
		});

		it("falls back to defaults when localStorage payload is malformed", () => {
			globalThis.localStorage?.setItem(
				TTS_PREFERENCES_STORAGE_KEY,
				"{not-valid-json",
			);
			const hydrated = createTtsPreferences();
			expect(hydrated.speed).toBe(1.0);
		});

		it("falls back to defaults when persisted speed is outside the choices", () => {
			globalThis.localStorage?.setItem(
				TTS_PREFERENCES_STORAGE_KEY,
				JSON.stringify({ speed: 9.9 }),
			);
			const hydrated = createTtsPreferences();
			expect(hydrated.speed).toBe(1.0);
		});
	});

	describe("SSR safety", () => {
		it("does not throw when localStorage is unavailable", () => {
			const originalStorage = globalThis.localStorage;
			// @ts-expect-error — deliberately remove for the SSR-style call
			delete globalThis.localStorage;
			try {
				const prefsSsr = createTtsPreferences();
				expect(prefsSsr.speed).toBe(1.0);
				expect(() => prefsSsr.setSpeed(1.25)).not.toThrow();
				expect(prefsSsr.speed).toBe(1.25);
			} finally {
				globalThis.localStorage = originalStorage;
			}
		});
	});

	describe("context key", () => {
		it("exports a unique Symbol for context key", () => {
			expect(typeof TTS_PREFERENCES_KEY).toBe("symbol");
		});
	});
});
