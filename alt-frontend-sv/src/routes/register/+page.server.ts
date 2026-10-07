import { redirect } from "@sveltejs/kit";
import { ory } from "#lib/ory.js";
import { sanitizeReturnTo } from "#lib/server/return-to.js";
import { KRATOS_PUBLIC_URL, ORIGIN } from "$app/env/private";
import type { PageServerLoad } from "./$types";

const basePath = "";

// /register への差し戻しループを防ぐための共通オプション
const REGISTER_RETURN_TO_OPTIONS = { loopPaths: ["/register"] };

export const load: PageServerLoad = async ({ url, locals, request }) => {
	const origin = ORIGIN;

	// If already logged in, redirect to home or return_to
	if (locals.session) {
		const returnTo = url.searchParams.get("return_to");
		const sanitizedReturnTo = sanitizeReturnTo(
			returnTo,
			origin,
			REGISTER_RETURN_TO_OPTIONS,
		);
		throw redirect(303, sanitizedReturnTo, { external: [origin] });
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
			`${KRATOS_PUBLIC_URL}/self-service/registration/browser`,
		);
		initUrl.searchParams.set("return_to", returnUrl);
		throw redirect(303, initUrl.toString(), { external: true });
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
