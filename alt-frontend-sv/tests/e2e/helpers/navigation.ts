import type { Page } from "@playwright/test";

const MOBILE_ROOT = process.env.MOBILE_BASE_PATH ?? "./mobile";

const normalizePath = (relative: string) => {
  const cleaned = relative
    .split("/")
    .map((segment) => segment.replace(/^\.+/, "").trim())
    .filter((segment) => segment.length > 0);

  if (cleaned[0] === "mobile") {
    cleaned.shift();
  }

  return cleaned.join("/");
};

export const mobileRoute = (relativePath: string) => {
  const normalized = normalizePath(relativePath);
  return normalized ? `${MOBILE_ROOT}/${normalized}` : MOBILE_ROOT;
};

export const gotoMobileRoute = (page: Page, relativePath: string) =>
  page.goto(mobileRoute(relativePath));
