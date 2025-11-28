"use server";

import { resolveServerSession } from "./server-session";
import {
  type BackendIdentityHeaders,
  buildBackendIdentityHeaders,
} from "./backend-headers";

/**
 * Gets server-side session headers for backend API calls.
 * This function resolves the session and returns headers including the backend token.
 * The backend token is never exposed to the client.
 */
export async function getServerSessionHeaders(): Promise<BackendIdentityHeaders | null> {
  const session = await resolveServerSession();
  if (!session) {
    return null;
  }

  return buildBackendIdentityHeaders({
    userId: session.user.id,
    email: session.user.email,
    role: session.user.role,
    sessionId: session.session.id,
    backendToken: session.backendToken,
  });
}
