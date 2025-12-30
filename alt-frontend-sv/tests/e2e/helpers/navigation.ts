import type { Page } from "@playwright/test";

const MOBILE_ROOT = process.env.MOBILE_BASE_PATH ?? "./mobile";
const DESKTOP_ROOT = process.env.DESKTOP_BASE_PATH ?? "./desktop";
const AUTH_ROOT = process.env.AUTH_BASE_PATH ?? ".";

/**
 * Normalize a relative path by removing leading dots and duplicate slashes.
 */
const normalizePath = (relative: string, prefixToRemove?: string) => {
	const cleaned = relative
		.split("/")
		.map((segment) => segment.replace(/^\.+/, "").trim())
		.filter((segment) => segment.length > 0);

	if (prefixToRemove && cleaned[0] === prefixToRemove) {
		cleaned.shift();
	}

	return cleaned.join("/");
};

// Mobile routes
export const mobileRoute = (relativePath: string) => {
	const normalized = normalizePath(relativePath, "mobile");
	return normalized ? `${MOBILE_ROOT}/${normalized}` : MOBILE_ROOT;
};

export const gotoMobileRoute = (page: Page, relativePath: string) =>
	page.goto(mobileRoute(relativePath));

// Desktop routes
export const desktopRoute = (relativePath: string) => {
	const normalized = normalizePath(relativePath, "desktop");
	return normalized ? `${DESKTOP_ROOT}/${normalized}` : DESKTOP_ROOT;
};

export const gotoDesktopRoute = (page: Page, relativePath: string) =>
	page.goto(desktopRoute(relativePath));

// Auth routes (no prefix)
export const authRoute = (relativePath: string) => {
	const normalized = normalizePath(relativePath);
	return `${AUTH_ROOT}/${normalized}`;
};

export const gotoAuthRoute = (page: Page, relativePath: string) =>
	page.goto(authRoute(relativePath));
