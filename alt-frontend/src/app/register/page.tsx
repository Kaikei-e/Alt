import { redirect } from "next/navigation";

export default function LegacyRegisterRedirect() {
  // Redirect to SvelteKit registration page for unified auth flow
  redirect("/sv/register");
}
