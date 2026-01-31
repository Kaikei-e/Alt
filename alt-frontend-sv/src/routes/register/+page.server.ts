import { redirect } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { ory } from "$lib/ory";
import type { PageServerLoad } from "./$types";

// KratosパブリックURL（ブラウザからのアクセス用）
const kratosPublicUrl = env.KRATOS_PUBLIC_URL || "http://localhost/ory";
// アプリケーションのベースURL
const appOrigin = env.ORIGIN || "http://localhost:4173";
const basePath = "/sv";

// 絶対URLかどうかをチェックするヘルパー関数
function isAbsoluteUrl(url: string): boolean {
	return /^https?:\/\//i.test(url);
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

	// /register を /sv/home に変換（ループ防止）
	if (cleanUrl.endsWith("/register") || cleanUrl.includes("/sv/register")) {
		return `${origin}/sv/home`;
	}

	return cleanUrl;
}

export const load: PageServerLoad = async ({ url, locals, request }) => {
	// If already logged in, redirect to home or return_to
	if (locals.session) {
		const returnTo = url.searchParams.get("return_to") || `${basePath}/`;
		throw redirect(303, returnTo);
	}

	const flow = url.searchParams.get("flow");
	const returnTo = url.searchParams.get("return_to");

	// If no flow, initiate registration flow
	if (!flow) {
		// return_toをサニタイズしてループを防ぐ（/sv/registerは/sv/homeに変換）
		const returnUrl = sanitizeReturnTo(returnTo, appOrigin);
		const initUrl = new URL(
			`${kratosPublicUrl}/self-service/registration/browser`,
		);
		initUrl.searchParams.set("return_to", returnUrl);
		throw redirect(303, initUrl.toString());
	}

	// Fetch flow data
	try {
		// クッキーを渡してflowを取得（Kratosはクッキーでセッションを検証）
		const cookie = request.headers.get("cookie") || undefined;
		const { data: flowData } = await ory.getRegistrationFlow({
			id: flow,
			cookie,
		});
		return {
			flow: flowData,
		};
	} catch (error) {
		// If flow is invalid or expired, redirect to error page to prevent infinite loop
		// フローが無効または期限切れの場合は、エラーページにリダイレクトしてループを防ぐ
		console.error("Failed to fetch registration flow:", error);

		const errorMessage = encodeURIComponent(
			"登録フローが無効または期限切れです。再度登録してください。",
		);
		throw redirect(303, `${basePath}/error?error=${errorMessage}`);
	}
};
