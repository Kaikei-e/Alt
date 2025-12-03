import { redirect } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { ory } from "$lib/ory";
import type { PageServerLoad } from "./$types";

// KratosパブリックURL（ブラウザからのアクセス用）
const kratosPublicUrl = env.KRATOS_PUBLIC_URL || "http://localhost/ory";
// アプリケーションのベースURL
const appOrigin = env.ORIGIN || "http://localhost:4173";
const basePath = "/sv";

export const load: PageServerLoad = async ({ url, locals }) => {
	// If already logged in, redirect to home or return_to
	if (locals.session) {
		const returnTo = url.searchParams.get("return_to") || `${basePath}/`;
		throw redirect(303, returnTo);
	}

	const flow = url.searchParams.get("flow");
	const returnTo = url.searchParams.get("return_to");

	// If no flow, initiate registration flow
	if (!flow) {
		// return_toが指定されていない場合は、現在の登録ページへのリダイレクトを設定
		const returnUrl = returnTo || `${appOrigin}${basePath}/register`;
		const initUrl = new URL(
			`${kratosPublicUrl}/self-service/registration/browser`,
		);
		initUrl.searchParams.set("return_to", returnUrl);
		throw redirect(303, initUrl.toString());
	}

	// Fetch flow data
	try {
		const { data: flowData } = await ory.getRegistrationFlow({ id: flow });
		return {
			flow: flowData,
		};
	} catch (error) {
		// If flow is invalid or expired, redirect to init
		const returnUrl = returnTo || `${appOrigin}${basePath}/register`;
		const initUrl = new URL(
			`${kratosPublicUrl}/self-service/registration/browser`,
		);
		initUrl.searchParams.set("return_to", returnUrl);
		throw redirect(303, initUrl.toString());
	}
};
