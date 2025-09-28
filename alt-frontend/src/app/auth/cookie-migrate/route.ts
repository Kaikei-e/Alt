import { NextResponse } from "next/server";

export async function GET(request: Request) {
  const responseHeaders = new Headers();

  const fallbackOrigin =
    process.env.NEXT_PUBLIC_APP_ORIGIN || "https://curionoah.com";

  let protocol = "https";
  try {
    protocol =
      request.headers.get("x-forwarded-proto") || new URL(request.url).protocol.replace(":", "");
  } catch {
    protocol = "https";
  }
  const isSecure = protocol === "https";

  let hostname: string;
  try {
    const originUrl = new URL(request.url);
    hostname = originUrl.hostname;
  } catch {
    hostname = new URL(fallbackOrigin).hostname;
  }

  const apexDomain =
    hostname === "localhost" || hostname === "127.0.0.1"
      ? hostname
      : `.${hostname}`;
  const secureFlag = isSecure ? "; Secure" : "";

  const buildCookie = (domain: string) =>
    `ory_kratos_session=; Max-Age=0; Path=/; Domain=${domain}; HttpOnly; SameSite=Lax${secureFlag}`;

  responseHeaders.append("Set-Cookie", buildCookie(hostname));
  responseHeaders.append("Set-Cookie", buildCookie(apexDomain));

  return new NextResponse(null, { status: 204, headers: responseHeaders });
}
