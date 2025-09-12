import { redirect } from "next/navigation";

export default function LegacyLoginRedirect() {
  // 308 Permanent Redirect to unified auth path
  redirect("/auth/login");
}
