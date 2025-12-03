import { ory } from "$lib/ory";
import { redirect } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = async ({ url, locals }) => {
  // If already logged in, redirect to home or return_to
  if (locals.session) {
    const returnTo = url.searchParams.get("return_to") || "/";
    throw redirect(303, returnTo);
  }

  const flow = url.searchParams.get("flow");
  const returnTo = url.searchParams.get("return_to");

  // If no flow, initiate registration flow
  if (!flow) {
    const kratosPublicUrl = "http://localhost:4433"; // TODO: Use env var
    const initUrl = new URL(`${kratosPublicUrl}/self-service/registration/browser`);
    if (returnTo) {
      initUrl.searchParams.set("return_to", returnTo);
    }
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
    const kratosPublicUrl = "http://localhost:4433";
    const initUrl = new URL(`${kratosPublicUrl}/self-service/registration/browser`);
    if (returnTo) {
      initUrl.searchParams.set("return_to", returnTo);
    }
    throw redirect(303, initUrl.toString());
  }
};
