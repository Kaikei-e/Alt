import { goto } from "$app/navigation";
import { Code, ConnectError } from "@connectrpc/connect";
import {
	createClientTransport,
	getLatestLetter,
	getLetterByDate,
	getLetterSources,
	getLetterEnrichment,
	regenerateLatestLetter,
} from "$lib/connect";
import type {
	MorningLetterDocument,
	MorningLetterSourceProto,
	MorningLetterBulletEnrichment,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";

/** Type guard for ConnectError Unauthenticated (handles both real and mocked instances). */
function isUnauthenticatedError(err: unknown): boolean {
	if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
		return true;
	}
	// Duck-type check for mocked ConnectError in tests
	if (
		err instanceof Error &&
		err.name === "ConnectError" &&
		"code" in err &&
		(err as Record<string, unknown>).code === 16
	) {
		return true;
	}
	return false;
}

export function useMorningLetter(initialLetter?: MorningLetterDocument | null) {
	// undefined = no data provided yet (need to fetch), null = explicitly no letter (NotFound)
	const hasInitialData = initialLetter !== undefined;
	let letter = $state<MorningLetterDocument | null>(initialLetter ?? null);
	let sources = $state<MorningLetterSourceProto[]>([]);
	let enrichments = $state<MorningLetterBulletEnrichment[]>([]);
	let letterLoading = $state(!hasInitialData);
	let sourcesLoading = $state(false);
	let enrichmentsLoading = $state(false);
	let error = $state<Error | null>(null);
	let enrichedForLetterId: string | null = null;
	let sourcedForLetterId: string | null = null;

	// When a Letter arrives (via SSR load data or a subsequent fetch),
	// auto-dispatch the sources + enrichment fan-outs exactly once per
	// distinct letter.id. Without this, a letter preloaded by +page.ts
	// would never trigger the enrichment RPC and the launch-pad deck
	// would silently stay empty.
	$effect(() => {
		const id = letter?.id;
		if (!id) return;
		if (sourcedForLetterId !== id) {
			sourcedForLetterId = id;
			void loadSources(id);
		}
		if (enrichedForLetterId !== id) {
			enrichedForLetterId = id;
			void loadEnrichments(id);
		}
	});

	const loadSources = async (letterId: string): Promise<void> => {
		sourcesLoading = true;
		try {
			const transport = createClientTransport();
			const result = await getLetterSources(transport, letterId);
			sources = result ?? [];
		} catch (err) {
			console.error("Failed to load letter sources:", err);
			sources = [];
		} finally {
			sourcesLoading = false;
		}
	};

	const loadEnrichments = async (letterId: string): Promise<void> => {
		enrichmentsLoading = true;
		try {
			const transport = createClientTransport();
			const result = await getLetterEnrichment(transport, letterId);
			enrichments = result ?? [];
		} catch (err) {
			console.error("Failed to load letter enrichment:", err);
			enrichments = [];
		} finally {
			enrichmentsLoading = false;
		}
	};

	const fetchLetter = async (): Promise<void> => {
		letterLoading = true;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await getLatestLetter(transport);
			letter = result;
			letterLoading = false;

			// Dispatch explicitly (keeps non-component test callers working).
			// The $effect above is a safety net for the initialLetter /
			// preloaded-via-+page.ts case where no method is ever called.
			// Its memo guard (*_ForLetterId) prevents double-fetch.
			if (result?.id) {
				sourcedForLetterId = result.id;
				enrichedForLetterId = result.id;
				void loadSources(result.id);
				void loadEnrichments(result.id);
			}
		} catch (err) {
			if (isUnauthenticatedError(err)) {
				goto("/login");
				return;
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			letter = null;
			letterLoading = false;
		}
	};

	const fetchByDate = async (date: string): Promise<void> => {
		letterLoading = true;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await getLetterByDate(transport, date);
			letter = result;
			letterLoading = false;

			if (result?.id) {
				sourcedForLetterId = result.id;
				enrichedForLetterId = result.id;
				void loadSources(result.id);
				void loadEnrichments(result.id);
			}
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
				goto("/login");
				return;
			}
			if (
				err instanceof Error &&
				err.name === "ConnectError" &&
				(err as { code?: number }).code === 16
			) {
				goto("/login");
				return;
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			letter = null;
			letterLoading = false;
		}
	};

	const retrySources = async (): Promise<void> => {
		if (letter?.id) {
			await loadSources(letter.id);
		}
	};

	let regenerating = $state(false);
	let regenerateCooldownMsg = $state<string | null>(null);

	const regenerate = async (
		editionTimezone?: string,
	): Promise<{ regenerated: boolean; retryAfterSeconds: number }> => {
		regenerating = true;
		regenerateCooldownMsg = null;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await regenerateLatestLetter(transport, editionTimezone);
			letter = result.letter;
			if (!result.regenerated && result.retryAfterSeconds > 0) {
				const minutes = Math.ceil(result.retryAfterSeconds / 60);
				regenerateCooldownMsg = `Using the cached edition — try again in ${minutes} minute${minutes === 1 ? "" : "s"}.`;
			}
			// sources + enrichments are dispatched by the $effect tracking letter.id
			return {
				regenerated: result.regenerated,
				retryAfterSeconds: result.retryAfterSeconds,
			};
		} catch (err) {
			if (isUnauthenticatedError(err)) {
				goto("/login");
				return { regenerated: false, retryAfterSeconds: 0 };
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			return { regenerated: false, retryAfterSeconds: 0 };
		} finally {
			regenerating = false;
		}
	};

	return {
		get letter() {
			return letter;
		},
		get sources() {
			return sources;
		},
		get enrichments() {
			return enrichments;
		},
		get letterLoading() {
			return letterLoading;
		},
		get sourcesLoading() {
			return sourcesLoading;
		},
		get enrichmentsLoading() {
			return enrichmentsLoading;
		},
		get error() {
			return error;
		},
		get regenerating() {
			return regenerating;
		},
		get regenerateCooldownMsg() {
			return regenerateCooldownMsg;
		},
		fetchLetter,
		fetchByDate,
		retrySources,
		regenerate,
	};
}
