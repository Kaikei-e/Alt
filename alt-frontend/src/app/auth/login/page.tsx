// app/auth/login/page.tsx
// Redirect to SvelteKit's /sv/auth/login for unified authentication

import { redirect } from "next/navigation";

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string; return_to?: string }>;
}) {
  const params = await searchParams;
  const returnTo = params?.return_to;

  // Redirect to SvelteKit's authentication page
  // Preserve return_to parameter if provided
  const redirectUrl = returnTo
    ? `/sv/auth/login?return_to=${encodeURIComponent(returnTo)}`
    : "/sv/auth/login";

  redirect(redirectUrl);
}
