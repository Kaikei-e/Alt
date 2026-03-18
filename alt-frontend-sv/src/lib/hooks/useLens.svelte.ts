/**
 * Hook for Lens (saved viewpoint) state management.
 */
import { ConnectError, Code } from "@connectrpc/connect";
import {
	createClientTransport,
	listLenses,
	createLens,
	deleteLens,
	selectLens,
} from "$lib/connect";
import type { LensData, LensVersionData } from "$lib/connect/knowledge_home";

export function useLens() {
	let lenses = $state<LensData[]>([]);
	let activeLensId = $state<string | null>(null);
	let loading = $state(false);
	let error = $state<Error | null>(null);

	const fetchLenses = async () => {
		try {
			loading = true;
			error = null;
			const transport = createClientTransport();
			const result = await listLenses(transport);
			lenses = result.lenses;
			activeLensId = result.activeLensId;
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.PermissionDenied) {
				lenses = [];
				return;
			}
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			loading = false;
		}
	};

	const create = async (name: string, description: string, version: Omit<LensVersionData, "versionId">) => {
		try {
			const transport = createClientTransport();
			const lens = await createLens(transport, name, description, version);
			if (lens) {
				lenses = [...lenses, lens];
			}
			return lens;
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
			return null;
		}
	};

	const remove = async (lensId: string) => {
		// Optimistic removal
		lenses = lenses.filter((l) => l.lensId !== lensId);
		if (activeLensId === lensId) {
			activeLensId = null;
		}
		try {
			const transport = createClientTransport();
			await deleteLens(transport, lensId);
		} catch {
			// Fire-and-forget
		}
	};

	const select = async (lensId: string | null) => {
		activeLensId = lensId;
		try {
			const transport = createClientTransport();
			await selectLens(transport, lensId);
		} catch {
			// Fire-and-forget
		}
	};

	const reset = () => {
		activeLensId = null;
	};

	return {
		get lenses() { return lenses; },
		get activeLensId() { return activeLensId; },
		get loading() { return loading; },
		get error() { return error; },
		fetchLenses,
		create,
		remove,
		select,
		reset,
	};
}
