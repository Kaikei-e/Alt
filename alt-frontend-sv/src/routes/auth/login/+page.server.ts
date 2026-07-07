import { redirect } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { ory } from "$lib/ory";
import { isAbsoluteUrl, sanitizeReturnTo } from "$lib/server/return-to";
import type { PageServerLoad } from "./$types";

// KratosパブリックURL（ブラウザからのアクセス用）
// 絶対URLである必要がある
const kratosPublicUrl = env.KRATOS_PUBLIC_URL || "http://localhost/ory";

// /login や /auth/login、bare "/" への差し戻しループを防ぐための共通オプション
const LOGIN_RETURN_TO_OPTIONS = { loopPaths: ["/login", "/auth/login", "/"] };

// KratosへのリダイレクトURLを生成するヘルパー関数
function buildKratosRedirectUrl(returnTo: string): string {
	// kratosPublicUrlが絶対URLであることを確認
	if (!isAbsoluteUrl(kratosPublicUrl)) {
		throw new Error(
			`KRATOS_PUBLIC_URL must be an absolute URL, got: ${kratosPublicUrl}`,
		);
	}

	const initUrl = new URL(`${kratosPublicUrl}/self-service/login/browser`);
	initUrl.searchParams.set("return_to", returnTo);
	return initUrl.toString();
}

export const load: PageServerLoad = async ({ url, locals, request }) => {
	// If already logged in, redirect to home or return_to
	if (locals.session) {
		const returnToParam = url.searchParams.get("return_to");
		const sanitizedReturnTo = sanitizeReturnTo(
			returnToParam,
			url.origin,
			LOGIN_RETURN_TO_OPTIONS,
		);
		throw redirect(303, sanitizedReturnTo);
	}

	const flow = url.searchParams.get("flow");
	const returnToParam = url.searchParams.get("return_to");

	// If no flow, initiate login flow with Kratos
	if (!flow) {
		// return_toをサニタイズしてループを防ぐ（/login・/auth/login・/ は /feeds に変換）
		const cleanUrl = sanitizeReturnTo(
			returnToParam,
			url.origin,
			LOGIN_RETURN_TO_OPTIONS,
		);
		const redirectUrl = buildKratosRedirectUrl(cleanUrl);
		throw redirect(303, redirectUrl);
	}

	// If flow exists, fetch and return flow data
	try {
		// クッキーを渡してflowを取得
		const cookie = request.headers.get("cookie") || undefined;
		const { data: flowData } = await ory.getLoginFlow({ id: flow, cookie });
		return {
			flow: flowData,
		};
	} catch (error) {
		// If flow is invalid or expired, redirect to error page to prevent infinite loop
		// フローが無効または期限切れの場合は、エラーページにリダイレクトしてループを防ぐ
		console.error("Failed to fetch login flow:", error);

		// 無限ループを防ぐため、エラーページにリダイレクト
		// ユーザーはエラーページから再度ログインを試みることができる
		const errorMessage = encodeURIComponent(
			"ログインフローが無効または期限切れです。再度ログインしてください。",
		);
		throw redirect(303, `/error?error=${errorMessage}`);
	}
};
