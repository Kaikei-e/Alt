import type { Identity } from "@ory/client";

/**
 * Auth Store - SSR-safe authentication state management
 *
 * Uses Svelte 5 runes for reactivity. Creates independent instances
 * via factory function to prevent state leakage in SSR.
 *
 * Best Practice (Svelte 5):
 * - Use createAuthStore() in root layout with setContext()
 * - Use getContext() in child components to access the store
 * - This ensures each request gets its own store instance in SSR
 */

export interface AuthStore {
	readonly user: Identity | null;
	readonly isAuthenticated: boolean;
	setUser(user: Identity | null): void;
	logout(): void;
}

/**
 * Internal state class using Svelte 5 runes
 */
class AuthStoreImpl implements AuthStore {
	private _user = $state<Identity | null>(null);

	get user(): Identity | null {
		return this._user;
	}

	get isAuthenticated(): boolean {
		return this._user !== null;
	}

	setUser(user: Identity | null): void {
		this._user = user;
	}

	logout(): void {
		this._user = null;
	}
}

/**
 * Factory function to create an AuthStore instance
 * Each call creates a new independent store - safe for SSR
 */
export function createAuthStore(): AuthStore {
	return new AuthStoreImpl();
}

// Context key for auth store
export const AUTH_STORE_KEY = Symbol("auth-store");

// Legacy singleton export for backward compatibility
// WARNING: Not SSR-safe - prefer createAuthStore() with context
export const auth = createAuthStore();
