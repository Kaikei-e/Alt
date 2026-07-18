import { redirect } from "@sveltejs/kit";
import { dev } from "$app/environment";
import { env } from "$env/dynamic/private";
import { ory } from "$lib/ory";
import { sanitizeReturnTo } from "$lib/server/return-to";
import type { PageServerLoad } from "./$types";

function requireEnv(name: string, fallbackDev: string): string {
	const value = env[name];
	if (value) return value;
	if (dev) return fallbackDev;
	throw new Error(`${name} must be set in production`);
}

// Resolve at request time — not module load. See auth/login/+page.server.ts.
function kratosPublicUrl(): string {
	return requireEnv("KRATOS_PUBLIC_URL", "http://localhost/ory");
}

function appOrigin(): string {
	return requireEnv("ORIGIN", "http://localhost:4173");
}

const basePath = "";

// /register への差し戻しループを防ぐための共通オプション
const REGISTER_RETURN_TO_OPTIONS = { loopPaths: ["/register"] };

export const load: PageServerLoad = async ({ url, locals, request }) => {
	const origin = appOrigin();

	// If already logged in, redirect to home or return_to
	if (locals.session) {
		const returnTo = url.searchParams.get("return_to");
		const sanitizedReturnTo = sanitizeReturnTo(
			returnTo,
			origin,
			REGISTER_RETURN_TO_OPTIONS,
		);
		throw redirect(303, sanitizedReturnTo);
	}

	const flow = url.searchParams.get("flow");
	const returnTo = url.searchParams.get("return_to");

	// If no flow, initiate registration flow
	if (!flow) {
		// return_toをサニタイズしてループを防ぐ（/registerは/feedsに変換）
		const returnUrl = sanitizeReturnTo(
			returnTo,
			origin,
			REGISTER_RETURN_TO_OPTIONS,
		);
		const initUrl = new URL(
			`${kratosPublicUrl()}/self-service/registration/browser`,
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
