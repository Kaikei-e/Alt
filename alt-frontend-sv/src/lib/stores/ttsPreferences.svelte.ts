import { getContext } from "svelte";

export const TTS_PREFERENCES_STORAGE_KEY = "alt:tts-preferences:v1";
export const TTS_PREFERENCES_KEY = Symbol("tts-preferences");

const SPEED_CHOICES = [0.75, 1.0, 1.25, 1.5, 2.0] as const;
const DEFAULT_SPEED = 1.0;

export type TtsSpeed = (typeof SPEED_CHOICES)[number];

export interface TtsPreferencesStore {
	readonly speed: number;
	readonly speedChoices: readonly number[];
	setSpeed(speed: number): void;
}

interface PersistedShape {
	speed?: number;
}

function readPersisted(): PersistedShape | null {
	try {
		const raw = globalThis.localStorage?.getItem(TTS_PREFERENCES_STORAGE_KEY);
		if (!raw) return null;
		const parsed = JSON.parse(raw);
		if (parsed && typeof parsed === "object") return parsed as PersistedShape;
		return null;
	} catch {
		return null;
	}
}

function writePersisted(value: PersistedShape): void {
	try {
		globalThis.localStorage?.setItem(
			TTS_PREFERENCES_STORAGE_KEY,
			JSON.stringify(value),
		);
	} catch {
		// localStorage may be unavailable (SSR, private mode); ignore.
	}
}

function isValidSpeed(value: unknown): value is TtsSpeed {
	return (
		typeof value === "number" &&
		(SPEED_CHOICES as readonly number[]).includes(value)
	);
}

class TtsPreferencesStoreImpl implements TtsPreferencesStore {
	private _speed = $state<number>(DEFAULT_SPEED);

	readonly speedChoices = SPEED_CHOICES as readonly number[];

	constructor() {
		const persisted = readPersisted();
		if (persisted && isValidSpeed(persisted.speed)) {
			this._speed = persisted.speed;
		}
	}

	get speed(): number {
		return this._speed;
	}

	setSpeed(speed: number): void {
		if (!isValidSpeed(speed)) {
			throw new Error(`Invalid TTS speed: ${speed}`);
		}
		this._speed = speed;
		writePersisted({ speed });
	}
}

export function createTtsPreferences(): TtsPreferencesStore {
	return new TtsPreferencesStoreImpl();
}

export function getTtsPreferences(): TtsPreferencesStore {
	return getContext<TtsPreferencesStore>(TTS_PREFERENCES_KEY);
}
