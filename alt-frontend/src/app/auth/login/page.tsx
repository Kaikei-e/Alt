// app/auth/login/page.tsx
import { redirect } from "next/navigation";
import { cookies } from "next/headers";
import LoginForm from "./LoginForm";

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string; return_to?: string }>;
}) {
  const params = await searchParams;
  const flow = params?.flow;
  const returnTo =
    params?.return_to ?? `${process.env.NEXT_PUBLIC_APP_ORIGIN}/`;

  // 🚨 FIX: Check if user is already logged in to prevent infinite loop
  // If session cookie exists, redirect to return_to instead of initiating login
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("ory_kratos_session") || cookieStore.get("ory-kratos-session");

  if (sessionCookie && !flow) {
    // User is already logged in, redirect to return_to
    redirect(returnTo);
  }

  if (!flow) {
    // flow がない場合は、return_toパラメータまたはデフォルトURLを使用
    const currentUrl =
      returnTo || `${process.env.NEXT_PUBLIC_APP_ORIGIN}/auth/login`;
    redirect(
      `${process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL}/self-service/login/browser?return_to=${encodeURIComponent(currentUrl)}`,
    );
  }

  // ここでは SSR で flow を取りにいかない（CORS/Cookie分離の罠を避ける）
  // UI はクライアントで取得（下の LoginForm.tsx）
  return <LoginForm flowId={flow} returnTo={returnTo} />;
}
