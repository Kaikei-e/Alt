// Centralized public env constants for client/runtime usage
// Use only static property access so Next can inline at build time.

type UrlValidationOptions = {
  allowPath?: boolean;
};

function assertPublicUrl(
  name: string,
  value: string | undefined,
  { allowPath = false }: UrlValidationOptions = {},
): string {
  if (!value) throw new Error(`${name} missing`);

  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`${name} must be a valid URL (got: ${value})`);
  }

  if (parsed.search || parsed.hash) {
    throw new Error(`${name} must not include query or hash fragments`);
  }

  // Allow HTTP in test/development environments or for localhost even in
  // production (local docker-compose).
  const isTestOrDev =
    process.env.NODE_ENV === "test" || process.env.NODE_ENV === "development";
  const hostIsLocal =
    parsed.hostname === "localhost" ||
    parsed.hostname === "127.0.0.1" ||
    parsed.hostname === "0.0.0.0";

  if (!isTestOrDev && parsed.protocol !== "https:" && !hostIsLocal) {
    throw new Error(
      `${name} must be HTTPS origin in production (got: ${parsed.protocol}//${parsed.host})`,
    );
  }

  if (
    (isTestOrDev && hostIsLocal) ||
    parsed.protocol === "https:" ||
    hostIsLocal
  ) {
    // Valid
  } else if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    throw new Error(
      `${name} must be a valid HTTP/HTTPS URL (got: ${parsed.protocol}//${parsed.host})`,
    );
  }

  const clusterLocalPattern = new RegExp(
    "\\." + "cluster" + "\\." + "local" + "(\\b|:|/)",
    "i",
  );
  if (!hostIsLocal && clusterLocalPattern.test(parsed.host)) {
    throw new Error(`${name} must be PUBLIC FQDN (got: ${parsed.host})`);
  }

  if (!allowPath && parsed.pathname !== "/") {
    throw new Error(
      `${name} must not include a path component (got: ${value})`,
    );
  }

  // SECURITY FIX: Remove identity replacement (replacing empty string with empty string has no effect)
  // Remove trailing slashes, but don't replace empty string with itself
  const sanitizedPath = allowPath
    ? parsed.pathname.replace(/\/+$/, "")
    : "";

  return `${parsed.origin}${sanitizedPath}`;
}

export const IDP_ORIGIN = assertPublicUrl(
  "NEXT_PUBLIC_IDP_ORIGIN",
  process.env.NEXT_PUBLIC_IDP_ORIGIN,
);

export const KRATOS_PUBLIC_URL = assertPublicUrl(
  "NEXT_PUBLIC_KRATOS_PUBLIC_URL",
  process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL,
  { allowPath: true },
);

// Optional public endpoints for CSP/connect-src etc.
export const PUBLIC_API_URL =
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
export const PUBLIC_WS_URL =
  process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080";
export const PUBLIC_API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:9000";
