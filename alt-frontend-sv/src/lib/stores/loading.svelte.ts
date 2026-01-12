/**
 * Loading Store - SSR-safe loading state management
 *
 * Used to show/hide the SystemLoader component during CSR data fetching.
 * Pages should set this to true when starting data fetch and false when complete.
 *
 * Best Practice (Svelte 5):
 * - Use createLoadingStore() in root layout with setContext()
 * - Use getContext() in child components to access the store
 * - This ensures each request gets its own store instance in SSR
 */

export interface LoadingStore {
	readonly isDesktopLoading: boolean;
	setLoading(loading: boolean): void;
	startLoading(): void;
	stopLoading(): void;
}

/**
 * Internal state class using Svelte 5 runes
 */
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
 * Factory function to create a LoadingStore instance
 * Each call creates a new independent store - safe for SSR
 */
export function createLoadingStore(): LoadingStore {
	return new LoadingStoreImpl();
}

// Context key for loading store
export const LOADING_STORE_KEY = Symbol("loading-store");

// Legacy singleton export for backward compatibility
// WARNING: Not SSR-safe - prefer createLoadingStore() with context
export const loadingStore = createLoadingStore();
