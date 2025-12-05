import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

/**
 * 7日間のリキャップデータを取得
 * バックエンドの/v1/recap/7daysエンドポイントにプロキシ
 * このエンドポイントは認証不要（publicly accessible）
 */
export const GET: RequestHandler = async ({ request }) => {
  try {
    const cookieHeader = request.headers.get("cookie") || "";

    // Forward headers
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    const controller = new AbortController();
    // 30秒タイムアウト（バックエンドがrecap-workerから取得するため長め）
    const timeoutId = setTimeout(() => controller.abort(), 30000);

    try {
      const backendResponse = await fetch(
        `${BACKEND_BASE_URL}/v1/recap/7days`,
        {
          method: "GET",
          headers: {
            Cookie: cookieHeader,
            "Content-Type": "application/json",
            "X-Forwarded-For": forwardedFor,
            "X-Forwarded-Proto": forwardedProto,
          },
          cache: "no-store",
          signal: controller.signal,
        },
      );

      clearTimeout(timeoutId);

      if (!backendResponse.ok) {
        const errorText = await backendResponse.text().catch(() => "");
        console.error(
          `Backend API error: ${backendResponse.status} ${backendResponse.statusText}`,
          {
            url: `${BACKEND_BASE_URL}/v1/recap/7days`,
            status: backendResponse.status,
            statusText: backendResponse.statusText,
            errorBody: errorText.substring(0, 200),
          },
        );

        // 404の場合はそのまま返す（リキャップがまだない場合）
        if (backendResponse.status === 404) {
          return json(
            { error: "No 7-day recap available yet" },
            { status: 404 },
          );
        }

        return json(
          {
            error: `Backend API error: ${backendResponse.status}`,
          },
          { status: backendResponse.status },
        );
      }

      const backendData = await backendResponse.json();
      return json(backendData);
    } catch (fetchError) {
      clearTimeout(timeoutId);
      if (fetchError instanceof Error && fetchError.name === "AbortError") {
        return json({ error: "Request timeout" }, { status: 504 });
      }
      throw fetchError;
    }
  } catch (error) {
    console.error("Error in /api/v1/recap/7days:", error);

    if (error instanceof Error && error.name === "AbortError") {
      return json({ error: "Request timeout" }, { status: 504 });
    }

    const errorMessage =
      error instanceof Error ? error.message : "Internal server error";
    return json({ error: errorMessage }, { status: 500 });
  }
};

