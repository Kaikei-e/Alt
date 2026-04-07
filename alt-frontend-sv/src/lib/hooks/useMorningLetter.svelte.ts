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

export function useMorningLetter(
	initialLetter: MorningLetterDocument | null = null,
) {
	let letter = $state<MorningLetterDocument | null>(initialLetter);
	let sources = $state<MorningLetterSourceProto[]>([]);
	let letterLoading = $state(!initialLetter);
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
			if (
				err instanceof ConnectError &&
				err.code === Code.Unauthenticated
			) {
				goto("/login");
				return;
			}
			// Duck-type check for mocked ConnectError
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
