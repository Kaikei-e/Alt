import type { Identity } from "@ory/client";
import { getContext } from "svelte";

export interface AuthStore {
	readonly user: Identity | null;
	readonly isAuthenticated: boolean;
	setUser(user: Identity | null): void;
	logout(): void;
}

class AuthStoreImpl implements AuthStore {
	private _user = $state<Identity | null>(null);

	readonly isAuthenticated = $derived(this._user !== null);

	get user(): Identity | null {
		return this._user;
	}

	setUser(user: Identity | null): void {
		this._user = user;
	}

	logout(): void {
		this._user = null;
	}
}

/**
 * Factory function to create an AuthStore instance.
 * Each call creates a new independent store - safe for SSR.
 */
export function createAuthStore(): AuthStore {
	return new AuthStoreImpl();
}

export const AUTH_STORE_KEY = Symbol("auth-store");

/**
 * Retrieve the AuthStore from Svelte context.
 * Must be called during component initialization (i.e. in <script> top level).
 */
export function getAuthStore(): AuthStore {
	return getContext<AuthStore>(AUTH_STORE_KEY);
}
