// app/_server/whoami.ts
import { cookies } from "next/headers";
export const dynamic = "force-dynamic";

export async function whoami() {
  const c = await cookies();
  const KRATOS_INTERNAL = process.env.KRATOS_INTERNAL_URL;
  if (!KRATOS_INTERNAL) throw new Error("KRATOS_INTERNAL_URL missing");
  return fetch(KRATOS_INTERNAL + "/sessions/whoami", {
    headers: { cookie: c.toString(), accept: "application/json" },
    cache: "no-store",
  });
}
