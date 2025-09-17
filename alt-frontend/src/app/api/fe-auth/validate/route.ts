import { headers } from "next/headers";

export async function GET() {
  const cookie = (await headers()).get("cookie") ?? "";
  const KRATOS =
    process.env.KRATOS_INTERNAL_URL ||
    process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL ||
    "https://id.curionoah.com";

  try {
    const r = await fetch(`${KRATOS}/sessions/whoami`, {
      headers: { cookie },
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    if (!r.ok) return Response.json({ ok: false }, { status: 401 });
    return Response.json({ ok: true }, { status: 200 });
  } catch (e: unknown) {
    const errorMessage = e instanceof Error ? e.message : 'Unknown error';
    return Response.json({ ok: false, error: errorMessage }, { status: 502 });
  }
}
