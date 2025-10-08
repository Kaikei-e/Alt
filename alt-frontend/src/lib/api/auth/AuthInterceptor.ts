export interface AuthInterceptorConfig {
  onAuthRequired?: () => void;
  recheckEndpoint?: string;
  recheckTimeout?: number;
  recheckStorageKey?: string;
}

const defaultConfig: Required<AuthInterceptorConfig> = {
  onAuthRequired: () => {},
  // Important: use Next.js local route handler to avoid reverse proxy capture of /api/auth/*
  // Nginx external proxies /api/auth/* to auth-service. Our FE recheck must hit Next directly.
  recheckEndpoint: "/api/fe-auth/validate",
  recheckTimeout: 3000,
  recheckStorageKey: "alt:recheck-whoami",
};

export class AuthInterceptor {
  private config: Required<AuthInterceptorConfig>;

  constructor(config: AuthInterceptorConfig = {}) {
    // Allow override via env (client-side safe NEXT_PUBLIC_*) if provided
    const envOverride =
      typeof window !== "undefined"
        ? (process.env.NEXT_PUBLIC_AUTH_RECHECK_ENDPOINT as string | undefined)
        : undefined;

    this.config = {
      ...defaultConfig,
      ...config,
      ...(envOverride ? { recheckEndpoint: envOverride } : {}),
    };
  }

  async intercept(
    response: Response,
    originalUrl: string,
    originalOptions?: RequestInit,
  ): Promise<Response> {
    if (response.status !== 401) {
      return response;
    }


    // Server-side execution check (critical for Next.js App Directory compatibility)
    if (typeof window === "undefined") {
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
          // ノイズ401ならリトライ - retry the original request
          return fetch(originalUrl, {
            ...originalOptions,
            credentials: "include",
          });
        }
      } catch (recheckError) {
      }
    }

    // ここで即遷移しない。ページ上部に「再ログインしてね」バナーを出すだけ。
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
