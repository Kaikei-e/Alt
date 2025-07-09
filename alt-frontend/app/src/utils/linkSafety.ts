import { safeUrlSchema } from "@/schema/validation/urlValidation";
import * as v from "valibot";

export interface LinkAttributes {
  rel?: string;
  target?: string;
  href: string;
}

export function sanitizeUrl(url: string): string {
  const result = v.safeParse(safeUrlSchema, url);
  if (!result.success) {
    // Return a safe fallback URL or empty string
    return "#";
  }
  return result.output;
}

export function addSecurityAttributes(url: string): LinkAttributes {
  const sanitizedUrl = sanitizeUrl(url);

  if (sanitizedUrl === "#") {
    return { href: "#" };
  }

  try {
    const parsedUrl = new URL(sanitizedUrl);
    const isExternal = parsedUrl.origin !== window.location.origin;

    return {
      href: sanitizedUrl,
      ...(isExternal && {
        rel: "noopener noreferrer",
        target: "_blank",
      }),
    };
  } catch {
    return { href: "#" };
  }
}

export function isExternalLink(url: string): boolean {
  try {
    const parsedUrl = new URL(url);
    return parsedUrl.origin !== window.location.origin;
  } catch {
    return false;
  }
}
