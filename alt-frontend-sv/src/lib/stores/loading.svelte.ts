import { getContext } from "svelte";

export interface LoadingStore {
	readonly isDesktopLoading: boolean;
	setLoading(loading: boolean): void;
	startLoading(): void;
	stopLoading(): void;
}

class LoadingStoreImpl implements LoadingStore {
	private _isDesktopLoading = $state(false);

	get isDesktopLoading(): boolean {
		return this._isDesktopLoading;
	}

	setLoading(loading: boolean): void {
		this._isDesktopLoading = loading;
	}

	startLoading(): void {
		this._isDesktopLoading = true;
	}

	stopLoading(): void {
		this._isDesktopLoading = false;
	}
}

/**
 * Factory function to create a LoadingStore instance.
 * Each call creates a new independent store - safe for SSR.
 */
export function createLoadingStore(): LoadingStore {
	return new LoadingStoreImpl();
}

export const LOADING_STORE_KEY = Symbol("loading-store");

/**
 * Retrieve the LoadingStore from Svelte context.
 * Must be called during component initialization (i.e. in <script> top level).
 */
export function getLoadingStore(): LoadingStore {
	return getContext<LoadingStore>(LOADING_STORE_KEY);
}
