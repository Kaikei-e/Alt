/**
 * Hook for playing AI summary speech.
 *
 * Manages playback state machine (idle | loading | playing | error | unavailable),
 * AudioContext gesture initialization, stream cancellation, and error latching.
 */

import type { Transport } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { createClientTransport } from "#lib/connect/index.js";
import {
	type SpeechChunk,
	type SynthesizeSpeechStreamOptions,
	synthesizeSpeechStream,
} from "#lib/connect/tts.js";
import {
	createSeamlessAudioPlayer,
	type SeamlessAudioPlayer,
} from "#lib/utils/seamlessAudioPlayer.js";

export type SummarySpeechState =
	| "idle"
	| "loading"
	| "playing"
	| "error"
	| "unavailable";

export interface SummarySpeechDeps {
	transport?: Transport;
	audioContext?: AudioContext;
	createAudioContext?: () => AudioContext;
	createPlayer?: (context: AudioContext) => SeamlessAudioPlayer;
	synthesizeStream?: (
		transport: Transport,
		options: SynthesizeSpeechStreamOptions,
		signal?: AbortSignal,
	) => AsyncIterable<SpeechChunk>;
}

export interface SummarySpeech {
	readonly state: SummarySpeechState;
	play(text: string): Promise<void> | void;
	stop(): void;
}

let sharedAudioContext: AudioContext | null = null;
let isUnavailableLatched = false;
let audioSessionConfigured = false;

interface NavigatorWithAudioSession {
	audioSession?: {
		type: string;
	};
}

function configureAudioSession(): void {
	if (audioSessionConfigured) return;
	audioSessionConfigured = true;
	if (typeof navigator !== "undefined" && "audioSession" in navigator) {
		const nav = navigator as unknown as NavigatorWithAudioSession;
		if (nav.audioSession) {
			nav.audioSession.type = "playback";
		}
	}
}

/** Test-only: resets shared AudioContext, latched unavailability, and audio session state. */
export function resetSummarySpeechSharedState(): void {
	sharedAudioContext = null;
	isUnavailableLatched = false;
	audioSessionConfigured = false;
}

function getSharedAudioContext(deps?: SummarySpeechDeps): AudioContext {
	if (deps?.audioContext) {
		return deps.audioContext;
	}
	if (sharedAudioContext?.state === "closed") {
		sharedAudioContext = null;
	}
	if (!sharedAudioContext) {
		if (deps?.createAudioContext) {
			sharedAudioContext = deps.createAudioContext();
		} else {
			const AudioContextCtor =
				window.AudioContext ||
				(window as unknown as { webkitAudioContext: typeof AudioContext })
					.webkitAudioContext;
			sharedAudioContext = new AudioContextCtor();
		}
	}
	return sharedAudioContext;
}

/**
 * Creates a reactive speech player controller for Visual Preview AI summaries.
 */
export function createSummarySpeech(deps?: SummarySpeechDeps): SummarySpeech {
	let state = $state<SummarySpeechState>(
		isUnavailableLatched ? "unavailable" : "idle",
	);
	let activePlayer: SeamlessAudioPlayer | null = null;
	let currentAbortController: AbortController | null = null;
	let currentPlayToken = 0;

	async function play(text: string): Promise<void> {
		if (isUnavailableLatched || state === "unavailable") {
			return;
		}

		// AudioContext must be resumed synchronously on user gesture
		const ctx = getSharedAudioContext(deps);
		configureAudioSession();
		void ctx.resume().catch(() => {
			sharedAudioContext = null;
		});

		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}
		if (activePlayer) {
			activePlayer.stop();
			activePlayer = null;
		}

		const token = ++currentPlayToken;
		const abortController = new AbortController();
		currentAbortController = abortController;

		state = "loading";

		const player = (deps?.createPlayer ?? createSeamlessAudioPlayer)(ctx);
		activePlayer = player;

		try {
			const transport = deps?.transport ?? createClientTransport();
			const streamFn = deps?.synthesizeStream ?? synthesizeSpeechStream;
			const stream = streamFn(transport, { text }, abortController.signal);

			for await (const chunk of stream) {
				if (token !== currentPlayToken || abortController.signal.aborted) {
					return;
				}
				state = "playing";
				await player.enqueue(chunk.audioWav);
			}

			if (token !== currentPlayToken || abortController.signal.aborted) {
				return;
			}

			await player.waitUntilDrained();

			if (token !== currentPlayToken || abortController.signal.aborted) {
				return;
			}

			state = "idle";
		} catch (err) {
			if (token !== currentPlayToken || abortController.signal.aborted) {
				return;
			}

			player.stop();
			activePlayer = null;

			const connectErr =
				err instanceof ConnectError ? err : ConnectError.from(err);
			if (connectErr.code === Code.FailedPrecondition) {
				isUnavailableLatched = true;
				state = "unavailable";
				return;
			}
			state = "error";
		} finally {
			if (token === currentPlayToken) {
				currentAbortController = null;
			}
		}
	}

	function stop(): void {
		if (state === "unavailable") {
			return;
		}
		currentPlayToken++;
		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}
		if (activePlayer) {
			activePlayer.stop();
			activePlayer = null;
		}
		state = "idle";
	}

	return {
		get state() {
			return state;
		},
		play,
		stop,
	};
}
