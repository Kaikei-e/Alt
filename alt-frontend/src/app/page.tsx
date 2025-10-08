import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export default async function RootPage() {
  // Check authentication status via session cookie
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("ory_kratos_session");

  if (sessionCookie?.value) {
    // Authenticated: redirect to home
    redirect("/home");
  } else {
    // Unauthenticated: redirect to landing page
    redirect("/public/landing");
  }
}
