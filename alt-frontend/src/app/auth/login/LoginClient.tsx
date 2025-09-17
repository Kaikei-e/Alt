"use client";
import { useEffect, useState } from "react";
import {
  Configuration,
  FrontendApi,
  UpdateLoginFlowBody,
  LoginFlow,
  UiNode,
} from "@ory/client";

const frontend = new FrontendApi(
  new Configuration({ basePath: process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL }),
);

// URL validation helper to prevent open redirects
const isValidReturnUrl = (url: string): boolean => {
  try {
    const parsedUrl = new URL(url);
    const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN;
    const idpOrigin = process.env.NEXT_PUBLIC_IDP_ORIGIN;

    // Only allow same-origin or trusted IDP origin redirects
    return parsedUrl.origin === appOrigin || parsedUrl.origin === idpOrigin;
  } catch {
    return false;
  }
};

// Safe redirect helper
const safeRedirect = (url: string) => {
  if (isValidReturnUrl(url)) {
    window.location.replace(url);
  } else {
    // Fallback to default safe URL
    window.location.replace(process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT!);
  }
};

export default function LoginClient() {
  const [flowId, setFlowId] = useState<string | null>(null);
  const [flow, setFlow] = useState<LoginFlow | null>(null);
  const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN!;
  const kratos = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL!;

  // 1) flowID 取得 or 新規作成
  useEffect(() => {
    const url = new URL(window.location.href);
    const f = url.searchParams.get("flow");
    if (!f) {
      const currentUrl = window.location.href;
      const returnTo = encodeURIComponent(currentUrl);
      const redirectUrl = `${kratos}/self-service/login/browser?return_to=${returnTo}`;
      safeRedirect(redirectUrl);
      return;
    }
    setFlowId(f);
  }, []);

  // 2) flow 読み込み（410はリダイレクトで復旧）
  useEffect(() => {
    if (!flowId) return;
    (async () => {
      try {
        const { data } = await frontend.getLoginFlow({ id: flowId });
        setFlow(data);
      } catch (e: unknown) {
        const error = e as {
          response?: { status?: number; data?: { error?: { id?: string } } };
          status?: number;
        };
        const status = error?.response?.status ?? error?.status;
        const errId = error?.response?.data?.error?.id;
        if (status === 410 || errId === "self_service_flow_expired") {
          const currentUrl = window.location.href.split("?")[0];
          const rt = encodeURIComponent(currentUrl);
          const redirectUrl = `${kratos}/self-service/login/browser?return_to=${rt}`;
          safeRedirect(redirectUrl);
          return;
        }
        // 既にセッションあり（session_already_available）なら既定へ
        if (errId === "session_already_available") {
          safeRedirect(process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT!);
          return;
        }
        console.error("getLoginFlow failed", e);
      }
    })();
  }, [flowId]);

  // 3) Submit（password例）
  async function onSubmit(ev: React.FormEvent<HTMLFormElement>) {
    ev.preventDefault();
    const form = new FormData(ev.currentTarget);
    const body: UpdateLoginFlowBody = {
      method: "password",
      identifier: String(form.get("identifier") || ""),
      password: String(form.get("password") || ""),
      csrf_token: String(form.get("csrf_token") || ""),
    };
    try {
      await frontend.updateLoginFlow({
        flow: flowId!,
        updateLoginFlowBody: body,
      });
      // 成功時は Kratos が Set-Cookie + リダイレクト/JSONを返す
      // ブラウザは `return_to` へ遷移
      // 念のためwhoamiで確認してから移動したければここでチェックしてもよい
      safeRedirect(process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT!);
    } catch (e) {
      console.error("updateLoginFlow failed", e);
      // TODO: エラーUI
    }
  }

  if (!flow) return <div>Loading…</div>;

  // Helper to safely get node attribute values
  const getNodeValue = (nodes: UiNode[], name: string): string => {
    const node = nodes.find(
      (n) =>
        n.attributes && "name" in n.attributes && n.attributes.name === name,
    );
    return node?.attributes && "value" in node.attributes
      ? node.attributes.value
      : "";
  };

  // 最小UI（nodesからCSRF/identifier/passwordを拾う前提の簡易例）
  const nodes = flow.ui?.nodes ?? [];
  const csrf = getNodeValue(nodes, "csrf_token");

  return (
    <form onSubmit={onSubmit}>
      <input type="hidden" name="csrf_token" value={csrf} />
      <input name="identifier" placeholder="email" />
      <input name="password" type="password" placeholder="password" />
      <button type="submit">Sign in</button>
    </form>
  );
}
