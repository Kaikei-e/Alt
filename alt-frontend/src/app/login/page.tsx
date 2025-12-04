import { redirect } from "next/navigation";

export default function LegacyLoginRedirect() {
  // Redirect to SvelteKit's unified auth path
  redirect("/sv/auth/login");
}
