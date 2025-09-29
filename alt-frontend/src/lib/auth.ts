export interface AuthValidateResponse {
  valid: boolean;
  session_id?: string;
  identity_id?: string;
}

export async function fetchAuth(): Promise<AuthValidateResponse> {
  // 🔧 修正: 正しいAPIパスに変更（v1/authを削除）
  const res = await fetch("/api/fe-auth/validate", {
    credentials: "include",
    headers: {
      "Cache-Control": "no-cache",
    },
  });

  if (res.status === 200) {
    const data = await res.json();
    const session = data?.session;
    const identity = session?.identity;
    return {
      valid: Boolean(data?.ok),
      session_id: session?.id,
      identity_id: identity?.id,
    };
  }

  if (res.status === 401) {
    return { valid: false };
  }

  // Other status codes are treated as service unavailable
  throw new Error(`auth validate unexpected: ${res.status}`);
}
