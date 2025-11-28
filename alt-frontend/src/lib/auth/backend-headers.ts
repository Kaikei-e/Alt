export type BackendIdentityHeaders = Record<string, string>;

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const isUuid = (value: unknown): value is string => {
  return typeof value === "string" && UUID_PATTERN.test(value);
};

export interface BackendIdentityHeadersOptions {
  userId: string;
  email: string;
  role: string;
  sessionId: string;
  backendToken: string;
}

/**
 * Builds backend identity headers including JWT token.
 * This function should only be called from server-side code.
 */
export function buildBackendIdentityHeaders(
  opts: BackendIdentityHeadersOptions,
): BackendIdentityHeaders | null {
  const { userId, email, role, sessionId, backendToken } = opts;

  if (!isUuid(userId)) {
    if (process.env.NODE_ENV === "development") {
      console.error("[buildBackendIdentityHeaders] Invalid userId:", {
        userId,
        isValidUserId: isUuid(userId),
      });
    }
    return null;
  }

  if (!sessionId || !sessionId.trim()) {
    if (process.env.NODE_ENV === "development") {
      console.error("[buildBackendIdentityHeaders] Missing sessionId");
    }
    return null;
  }

  if (!backendToken || !backendToken.trim()) {
    if (process.env.NODE_ENV === "development") {
      console.error("[buildBackendIdentityHeaders] Missing backendToken");
    }
    return null;
  }

  const headers: BackendIdentityHeaders = {
    "X-Alt-User-Id": userId,
    "X-Alt-Tenant-Id": userId, // Single-tenant: use userId as tenantId
    "X-Alt-User-Email": email,
    "X-Alt-User-Role": role || "user",
    "X-Alt-Session-Id": sessionId.trim(),
    "X-Alt-Backend-Token": backendToken.trim(),
  };

  return headers;
}
