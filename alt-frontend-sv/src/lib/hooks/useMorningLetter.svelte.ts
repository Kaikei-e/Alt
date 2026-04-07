import { goto } from "$app/navigation";
import { Code, ConnectError } from "@connectrpc/connect";
import {
	createClientTransport,
	getLatestLetter,
	getLetterByDate,
	getLetterSources,
} from "$lib/connect";
import type {
	MorningLetterDocument,
	MorningLetterSourceProto,
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
		(err as Record<string, unknown>)["code"] === 16
	) {
		return true;
	}
	return false;
}

export function useMorningLetter(
	initialLetter?: MorningLetterDocument | null,
) {
	// undefined = no data provided yet (need to fetch), null = explicitly no letter (NotFound)
	const hasInitialData = initialLetter !== undefined;
	let letter = $state<MorningLetterDocument | null>(initialLetter ?? null);
	let sources = $state<MorningLetterSourceProto[]>([]);
	let letterLoading = $state(!hasInitialData);
	let sourcesLoading = $state(false);
	let error = $state<Error | null>(null);

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

	const fetchLetter = async (): Promise<void> => {
		letterLoading = true;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await getLatestLetter(transport);
			letter = result;
			letterLoading = false;

			if (result?.id) {
				void loadSources(result.id);
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
				void loadSources(result.id);
			}
		} catch (err) {
			if (
				err instanceof ConnectError &&
				err.code === Code.Unauthenticated
			) {
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

	return {
		get letter() {
			return letter;
		},
		get sources() {
			return sources;
		},
		get letterLoading() {
			return letterLoading;
		},
		get sourcesLoading() {
			return sourcesLoading;
		},
		get error() {
			return error;
		},
		fetchLetter,
		fetchByDate,
		retrySources,
	};
}
