/**
 * Loading Store Tests
 *
 * Tests for SSR-safe loading state management using Svelte 5 runes
 */
import { describe, expect, it, beforeEach } from "vitest";
import { createLoadingStore, type LoadingStore } from "./loading.svelte";

describe("LoadingStore", () => {
	let store: LoadingStore;

	beforeEach(() => {
		store = createLoadingStore();
	});

	describe("initial state", () => {
		it("should have isDesktopLoading as false by default", () => {
			expect(store.isDesktopLoading).toBe(false);
		});
	});

	describe("setLoading", () => {
		it("should set loading state to true", () => {
			store.setLoading(true);
			expect(store.isDesktopLoading).toBe(true);
		});

		it("should set loading state to false", () => {
			store.setLoading(true);
			store.setLoading(false);
			expect(store.isDesktopLoading).toBe(false);
		});
	});

	describe("startLoading", () => {
		it("should set loading state to true", () => {
			store.startLoading();
			expect(store.isDesktopLoading).toBe(true);
		});
	});

	describe("stopLoading", () => {
		it("should set loading state to false", () => {
			store.startLoading();
			store.stopLoading();
			expect(store.isDesktopLoading).toBe(false);
		});
	});

	describe("SSR safety", () => {
		it("should create independent instances for each call", () => {
			const store1 = createLoadingStore();
			const store2 = createLoadingStore();

			store1.startLoading();

			// store2 should not be affected by store1
			expect(store1.isDesktopLoading).toBe(true);
			expect(store2.isDesktopLoading).toBe(false);
		});
	});
});
