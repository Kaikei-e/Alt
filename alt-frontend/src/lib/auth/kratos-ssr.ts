import { GetServerSidePropsContext, GetServerSidePropsResult } from "next";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";

export interface AuthUser {
  id: string;
  email: string;
  name?: string;
}

export interface AuthSession {
  identity: AuthUser;
  authenticated_at: string;
  expires_at: string;
}

/**
 * SSR authentication guard using Kratos whoami endpoint
 * TODO.mdに従った実装: サーバーサイドでwhoamiを見て、未ログインならリダイレクト
 */
export async function withAuth<
  P extends Record<string, any> = Record<string, any>,
>(
  getServerSideProps?: (
    context: GetServerSidePropsContext,
    user: AuthUser,
  ) => Promise<GetServerSidePropsResult<P>> | GetServerSidePropsResult<P>,
): Promise<
  (
    context: GetServerSidePropsContext,
  ) => Promise<GetServerSidePropsResult<P | {}>>
> {
  return async (
    context: GetServerSidePropsContext,
  ): Promise<GetServerSidePropsResult<P | {}>> => {
    const { req } = context;

    try {
      // TODO.mdの要求：サーバサイドでwhoamiを呼び出し、必ずCookieを前方転送
      const KRATOS_INTERNAL =
        process.env.KRATOS_INTERNAL_URL || "http://localhost:4433";
      const response = await fetch(`${KRATOS_INTERNAL}/sessions/whoami`, {
        headers: {
          cookie: req.headers.cookie ?? "", // 必ずCookieを前方転送
          Accept: "application/json",
        },
        cache: "no-store", // TODO.md修正: サーバ側fetch個別キャッシュ防止
      });

      // TODO.mdの要求：未ログインならリダイレクト
      if (response.status !== 200) {
        const currentPath = context.resolvedUrl || context.req.url || "/";
        const abs = new URL(
          currentPath,
          `http://${context.req.headers.host || "localhost"}`,
        ).toString();
        const loginUrl = `/auth/login?return_to=${encodeURIComponent(abs)}`;

        return {
          redirect: {
            destination: loginUrl,
            permanent: false,
          },
        };
      }

      const session: AuthSession = await response.json();

      if (!session.identity) {
        const currentPath = context.resolvedUrl || context.req.url || "/";
        const abs = new URL(
          currentPath,
          `http://${context.req.headers.host || "localhost"}`,
        ).toString();
        const loginUrl = `/auth/login?return_to=${encodeURIComponent(abs)}`;

        return {
          redirect: {
            destination: loginUrl,
            permanent: false,
          },
        };
      }

      // 認証済みの場合、元のgetServerSidePropsを実行
      if (getServerSideProps) {
        return await getServerSideProps(context, session.identity);
      }

      // デフォルトではユーザー情報をpropsで渡す
      return {
        props: {
          user: session.identity,
        } as unknown as P,
      };
    } catch (error) {
      console.error("Authentication check failed:", error);

      // エラーの場合もログインページにリダイレクト
      const currentPath = context.resolvedUrl || context.req.url || "/";
      const abs = new URL(
        currentPath,
        `http://${context.req.headers.host || "localhost"}`,
      ).toString();
      const loginUrl = `/auth/login?return_to=${encodeURIComponent(abs)}`;

      return {
        redirect: {
          destination: loginUrl,
          permanent: false,
        },
      };
    }
  };
}

/**
 * Client-side authentication check using Kratos whoami
 */
export async function checkAuthStatus(): Promise<AuthUser | null> {
  try {
    // クライアント側は公開URLを使用（ブラウザからのアクセス）
    const response = await fetch(`${KRATOS_PUBLIC_URL}/sessions/whoami`, {
      credentials: "include",
      cache: "no-store", // TODO.md要件: 常時no-store
      headers: {
        Accept: "application/json",
      },
    });

    if (response.status !== 200) {
      return null;
    }

    const session: AuthSession = await response.json();
    return session.identity || null;
  } catch (error) {
    console.error("Client auth check failed:", error);
    return null;
  }
}

/**
 * Logout function that calls Kratos logout endpoint
 */
export async function logout(): Promise<void> {
  try {
    const response = await fetch(
      `${KRATOS_PUBLIC_URL}/self-service/logout/browser`,
      {
        method: "GET",
        credentials: "include",
        cache: "no-store", // TODO.md修正: サーバ側fetch個別キャッシュ防止
      },
    );

    if (response.status === 303) {
      // Follow redirect to complete logout
      const location = response.headers.get("Location");
      if (location) {
        window.location.href = location;
        return;
      }
    }

    // Fallback: redirect to app login route
    const currentUrl =
      typeof window !== "undefined" ? window.location.href : "/";
    window.location.href = `/auth/login?return_to=${encodeURIComponent(currentUrl)}`;
  } catch (error) {
    console.error("Logout failed:", error);
    // Force redirect to login even on error
    const currentUrl =
      typeof window !== "undefined" ? window.location.href : "/";
    window.location.href = `/auth/login?return_to=${encodeURIComponent(currentUrl)}`;
  }
}
