import type { User } from "@/types/auth";

export type BackendIdentityHeaders = Record<string, string>;

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const isUuid = (value: unknown): value is string => {
  return typeof value === "string" && UUID_PATTERN.test(value);
};

export const buildBackendIdentityHeaders = (
  user: User | null | undefined,
  sessionId: string | null | undefined,
): BackendIdentityHeaders | null => {
  if (!user) return null;

  const { id, tenantId, email, role } = user;

  if (!isUuid(id) || !isUuid(tenantId)) {
    return null;
  }

  const headers: BackendIdentityHeaders = {
    "X-Alt-User-Id": id,
    "X-Alt-Tenant-Id": tenantId,
  };

  if (email) {
    headers["X-Alt-User-Email"] = email;
  }

  if (role) {
    headers["X-Alt-User-Role"] = role;
  }

  if (sessionId && sessionId.trim()) {
    headers["X-Alt-Session-Id"] = sessionId.trim();
  }

  return headers;
};

