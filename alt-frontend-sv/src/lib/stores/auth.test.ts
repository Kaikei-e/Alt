/**
 * Auth Store Tests
 *
 * Tests for SSR-safe auth state management using Svelte 5 runes
 */
import { describe, expect, it, beforeEach } from "vitest";
import { createAuthStore, type AuthStore } from "./auth.svelte";

describe("AuthStore", () => {
	let authStore: AuthStore;

	beforeEach(() => {
		authStore = createAuthStore();
	});

	describe("initial state", () => {
		it("should have null user by default", () => {
			expect(authStore.user).toBeNull();
		});

		it("should have isAuthenticated as false by default", () => {
			expect(authStore.isAuthenticated).toBe(false);
		});
	});

	describe("setUser", () => {
		it("should set user and update isAuthenticated to true", () => {
			const mockUser = {
				id: "user-123",
				traits: { email: "test@example.com" },
			} as any;

			authStore.setUser(mockUser);

			expect(authStore.user).toBe(mockUser);
			expect(authStore.isAuthenticated).toBe(true);
		});

		it("should set user to null and update isAuthenticated to false", () => {
			const mockUser = {
				id: "user-123",
				traits: { email: "test@example.com" },
			} as any;

			authStore.setUser(mockUser);
			authStore.setUser(null);

			expect(authStore.user).toBeNull();
			expect(authStore.isAuthenticated).toBe(false);
		});
	});

	describe("SSR safety", () => {
		it("should create independent instances for each call", () => {
			const store1 = createAuthStore();
			const store2 = createAuthStore();

			const mockUser = { id: "user-1" } as any;
			store1.setUser(mockUser);

			// store2 should not be affected by store1
			expect(store1.user?.id).toBe("user-1");
			expect(store2.user).toBeNull();
		});
	});

	describe("logout", () => {
		it("should clear user and set isAuthenticated to false", () => {
			const mockUser = { id: "user-123" } as any;
			authStore.setUser(mockUser);

			authStore.logout();

			expect(authStore.user).toBeNull();
			expect(authStore.isAuthenticated).toBe(false);
		});
	});
});
