import { redirect } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { ory } from "$lib/ory";
import type { PageServerLoad } from "./$types";

// KratosパブリックURL（ブラウザからのアクセス用）
// 絶対URLである必要がある
const kratosPublicUrl = env.KRATOS_PUBLIC_URL || "http://localhost/ory";

// 絶対URLかどうかをチェックするヘルパー関数
function isAbsoluteUrl(url: string): boolean {
	return /^https?:\/\//i.test(url);
}

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

// return_toをサニタイズして、ループを防ぐ
function sanitizeReturnTo(returnTo: string | null, origin: string): string {
	if (!returnTo) {
		return `${origin}/sv/home`;
	}

	// 絶対URLの場合はそのまま使用、相対URLの場合はoriginを追加
	let cleanUrl: string;
	if (isAbsoluteUrl(returnTo)) {
		cleanUrl = returnTo;
	} else {
		cleanUrl = `${origin}${returnTo.startsWith("/") ? returnTo : `/${returnTo}`}`;
	}

	// return_toからクエリパラメータを削除（ループを防ぐため）
	cleanUrl = cleanUrl.split("?")[0];

	// /login や /auth/login を /sv/home に変換（ループを防ぐ）
	if (cleanUrl.endsWith("/login") || cleanUrl.endsWith("/auth/login")) {
		return `${origin}/sv/home`;
	}

	// /sv/auth/login や /sv/login や /sv/ の場合は /sv/home にリダイレクト（ループを防ぐ）
	if (
		cleanUrl.includes("/sv/auth/login") ||
		cleanUrl.includes("/sv/login") ||
		cleanUrl.endsWith("/sv/") ||
		cleanUrl === `${origin}/sv`
	) {
		return `${origin}/sv/home`;
	}

	return cleanUrl;
}

export const load: PageServerLoad = async ({ url, locals, request }) => {
	// If already logged in, redirect to home or return_to
	if (locals.session) {
		const returnToParam = url.searchParams.get("return_to");
		const sanitizedReturnTo = sanitizeReturnTo(returnToParam, url.origin);
		throw redirect(303, sanitizedReturnTo);
	}

	const flow = url.searchParams.get("flow");
	const returnToParam = url.searchParams.get("return_to");

	// If no flow, initiate login flow with Kratos
	if (!flow) {
		// return_toが指定されていない場合は、/sv/home を使用
		// /sv/auth/login や /sv/login や /sv/ の場合は /sv/home に変更（ループを防ぐ）
		let cleanUrl = returnToParam || `${url.origin}/sv/home`;

		// 絶対URLの場合はそのまま使用、相対URLの場合はoriginを追加
		if (!isAbsoluteUrl(cleanUrl)) {
			cleanUrl = `${url.origin}${cleanUrl.startsWith("/") ? cleanUrl : `/${cleanUrl}`}`;
		}

		// return_toからクエリパラメータを削除（ループを防ぐため）
		cleanUrl = cleanUrl.split("?")[0];

		// /login や /auth/login を /sv/home に変換（ループを防ぐ）
		if (cleanUrl.endsWith("/login") || cleanUrl.endsWith("/auth/login")) {
			cleanUrl = `${url.origin}/sv/home`;
		} else {
			// /auth/login を /sv/home に変換
			cleanUrl = cleanUrl.replace("/auth/login", "/sv/home");
		}

		// /sv/auth/login や /sv/login や /sv/ を含む場合は /sv/home に変更（ループを防ぐ）
		if (
			cleanUrl.includes("/sv/auth/login") ||
			cleanUrl.includes("/sv/login") ||
			cleanUrl.endsWith("/sv/") ||
			cleanUrl === `${url.origin}/sv`
		) {
			cleanUrl = `${url.origin}/sv/home`;
		}

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
		throw redirect(303, `/sv/error?error=${errorMessage}`);
	}
};
