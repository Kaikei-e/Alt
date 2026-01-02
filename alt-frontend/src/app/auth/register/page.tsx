// app/auth/register/page.tsx
// Redirect to SvelteKit registration page for unified auth flow
import { redirect } from "next/navigation";

export default async function RegisterPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string>>;
}) {
  const params = await searchParams;
  const returnTo = params.return_to;

  // Build redirect URL to SvelteKit registration page
  const svRegisterUrl = new URL("/sv/register", process.env.NEXT_PUBLIC_APP_ORIGIN || "https://curionoah.com");
  if (returnTo) {
    svRegisterUrl.searchParams.set("return_to", returnTo);
  }

  redirect(svRegisterUrl.toString());
}
