export interface AuthInterceptorConfig {
  onAuthRequired?: () => void;
  recheckEndpoint?: string;
  recheckTimeout?: number;
  recheckStorageKey?: string;
}

const defaultConfig: Required<AuthInterceptorConfig> = {
  onAuthRequired: () => {},
  recheckEndpoint: "/api/auth/recheck",
  recheckTimeout: 3000,
  recheckStorageKey: "alt:recheck-whoami",
};

export class AuthInterceptor {
  private config: Required<AuthInterceptorConfig>;

  constructor(config: AuthInterceptorConfig = {}) {
    this.config = { ...defaultConfig, ...config };
  }

  async intercept(
    response: Response,
    originalUrl: string,
    originalOptions?: RequestInit,
  ): Promise<Response> {
    if (response.status !== 401) {
      return response;
    }

    console.warn("[AUTH] 401 Unauthorized detected");

    // Server-side execution check (critical for Next.js App Directory compatibility)
    if (typeof window === "undefined") {
      console.warn(
        "[AUTH] Server-side 401 detected, letting error propagate for SSR",
      );
      return response;
    }

    // セッション確認を一回だけ（SSRは既にやっているが、ネットワーク一過性対策）
    if (!sessionStorage.getItem(this.config.recheckStorageKey)) {
      sessionStorage.setItem(this.config.recheckStorageKey, "1");

      try {
        const recheckResponse = await fetch(this.config.recheckEndpoint, {
          credentials: "include",
          signal: AbortSignal.timeout(this.config.recheckTimeout),
        });

        if (recheckResponse.ok) {
          console.log("[AUTH] Recheck succeeded, retrying original request");
          // ノイズ401ならリトライ - retry the original request
          return fetch(originalUrl, {
            ...originalOptions,
            credentials: "include",
          });
        }
      } catch (recheckError) {
        console.warn("[AUTH] Recheck failed:", recheckError);
      }
    }

    // ここで即遷移しない。ページ上部に「再ログインしてね」バナーを出すだけ。
    console.log("[AUTH] Showing login banner instead of redirecting");
    this.config.onAuthRequired();

    // Return a special response to indicate authentication issue without redirect
    return new Response(
      JSON.stringify({
        authRequired: true,
        message: "Authentication required - login banner shown",
      }),
      {
        status: 401,
        headers: { "Content-Type": "application/json" },
      },
    );
  }
}
