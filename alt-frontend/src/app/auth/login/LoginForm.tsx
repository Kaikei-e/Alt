// app/auth/login/LoginForm.tsx（Client Componentの一例）
"use client";
import { useState, useEffect } from "react";
import {
  Configuration,
  FrontendApi,
  UpdateLoginFlowBody,
  LoginFlow,
  UiNode,
} from "@ory/client";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";

const kratos = new FrontendApi(
  new Configuration({
    basePath: "/ory",
    baseOptions: { credentials: "include" }, // ★必須（AJAXでCookieを送る）
  }),
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
    window.location.href = url;
  } else {
    // Fallback to default safe URL
    window.location.href = process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT || "/";
  }
};

// Custom interface for error states not covered by Ory types
interface LoginFlowError {
  error: boolean;
  message: string;
}

// Type guard to check if flow is a valid LoginFlow
const isLoginFlow = (
  flow: LoginFlow | LoginFlowError | null,
): flow is LoginFlow => {
  return flow !== null && "ui" in flow && !("error" in flow);
};

export default function LoginForm({ flowId }: { flowId: string }) {
  const [flow, setFlow] = useState<LoginFlow | LoginFlowError | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [useNativeForm, setUseNativeForm] = useState(false);
  const defaultReturnTo =
    process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT || "https://curionoah.com/";
  const recoveryUrl = `${KRATOS_PUBLIC_URL}/self-service/recovery/browser?return_to=${encodeURIComponent(
    defaultReturnTo,
  )}`;
  const registrationUrl = `${KRATOS_PUBLIC_URL}/self-service/registration/browser?return_to=${encodeURIComponent(
    defaultReturnTo,
  )}`;

  // 1) SDK で flow を取得して nodes を描画
  // 2) 送信は SDK or ネイティブ <form> のどちらでも切替可（検証しやすくする）
  useEffect(() => {
    kratos
      .getLoginFlow({ id: flowId })
      .then((response) => setFlow(response.data))
      .catch((err) => {
        console.error("Failed to load login flow:", err);
        const status = err.response?.status ?? err?.status;
        // 410の場合は新しいフローに再リダイレクト
        if (status === 410) {
          const currentUrl = window.location.href;
          const redirectUrl = `/ory/self-service/login/browser?return_to=${encodeURIComponent(currentUrl)}`;
          safeRedirect(redirectUrl);
        }
        // 403 CSRF エラーの場合も新しいフローに再リダイレクト
        else if (status === 403) {
          console.warn("CSRF error detected, creating new login flow");
          const currentUrl = window.location.href;
          const redirectUrl = `/ory/self-service/login/browser?return_to=${encodeURIComponent(currentUrl)}`;
          safeRedirect(redirectUrl);
        }
        // その他のエラーの場合はエラー状態を設定
        else {
          setFlow({
            error: true,
            message: "Failed to load login form. Please try again.",
          });
        }
      });
  }, [flowId]);

  if (!flow) {
    return (
      <div className="login-container">
        <div className="login-card glass">
          <div className="loading-spinner">
            <div className="spinner"></div>
            <p>Loading...</p>
          </div>
        </div>

        <style jsx>{`
          .login-container {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: var(--space-4);
          }

          .login-card {
            width: 100%;
            max-width: 400px;
            padding: var(--space-8);
            margin: var(--space-4);
          }

          .loading-spinner {
            text-align: center;
          }

          .spinner {
            width: 32px;
            height: 32px;
            border: 3px solid var(--surface-border);
            border-top: 3px solid var(--alt-primary);
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto var(--space-4) auto;
          }

          @keyframes spin {
            0% {
              transform: rotate(0deg);
            }
            100% {
              transform: rotate(360deg);
            }
          }
        `}</style>
      </div>
    );
  }

  if ("error" in flow && flow.error) {
    return (
      <div className="login-container">
        <div className="login-card glass">
          <div className="error-state">
            <div className="error-icon">⚠️</div>
            <h2>Authentication Error</h2>
            <p>{flow.message}</p>
            <button
              onClick={() => window.location.reload()}
              className="btn-accent"
            >
              Try Again
            </button>
          </div>
        </div>

        <style jsx>{`
          .login-container {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: var(--space-4);
          }

          .login-card {
            width: 100%;
            max-width: 400px;
            padding: var(--space-8);
            margin: var(--space-4);
          }

          .error-state {
            text-align: center;
          }

          .error-icon {
            font-size: var(--text-4xl);
            margin-bottom: var(--space-4);
          }

          .error-state h2 {
            color: var(--alt-error);
            margin-bottom: var(--space-2);
          }

          .error-state p {
            color: var(--text-muted);
            margin-bottom: var(--space-6);
          }

          .btn-accent {
            padding: var(--space-3) var(--space-6);
            background: var(--accent-gradient);
            border: none;
            border-radius: var(--radius-full);
            color: white;
            font-weight: 600;
            cursor: pointer;
            transition: all var(--transition-speed) ease;
          }

          .btn-accent:hover {
            transform: translateY(-2px);
            filter: brightness(1.1);
          }
        `}</style>
      </div>
    );
  }

  // Ensure flow has ui property before accessing nodes
  if (!isLoginFlow(flow)) {
    return <div>Loading...</div>;
  }

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

  const nodes = flow.ui?.nodes ?? [];
  const csrf = getNodeValue(nodes, "csrf_token");

  // --- SDK 送信用 ---
  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSubmitting(true);
    const formData = new FormData(e.currentTarget);
    const body = {
      ...Object.fromEntries(formData.entries()),
      method: "password", // ★ 必須: Kratos認証戦略指定
    } as unknown as UpdateLoginFlowBody;

    console.log("Login form submission:", {
      flowId,
      body: { ...body, password: "***" },
    });
    try {
      const res = await kratos.updateLoginFlow({
        flow: flowId,
        updateLoginFlowBody: body,
      });
      // 成功時：303 + Set-Cookie（ブラウザが遷移 or JSON / redirect_to）
      const responseData = res.data as {
        redirect_to?: string;
        return_to?: string;
      };
      const redirectTo = responseData.redirect_to || responseData.return_to;
      safeRedirect(redirectTo ?? "/");
    } catch (err: unknown) {
      const error = err as {
        response?: { status?: number; data?: { error?: { id?: string } } };
        status?: number;
      };
      const st = error?.response?.status ?? error?.status;
      const id = error?.response?.data?.error?.id;

      // 410なら自動復帰
      if (st === 410 || id === "self_service_flow_expired") {
        const currentUrl = window.location.href;
        const redirectUrl = `/ory/self-service/login/browser?return_to=${encodeURIComponent(currentUrl)}`;
        safeRedirect(redirectUrl);
        return;
      }

      // 403 CSRF エラーも自動復帰
      if (st === 403) {
        console.warn("CSRF error during login submission, creating new flow");
        const currentUrl = window.location.href;
        const redirectUrl = `/ory/self-service/login/browser?return_to=${encodeURIComponent(currentUrl)}`;
        safeRedirect(redirectUrl);
        return;
      }

      console.error("updateLoginFlow failed", err);
      setSubmitting(false);
    }
  }

  // --- ネイティブフォーム（最小経路・検証用） ---
  // flow.ui.action / flow.ui.nodes を使ってそのまま form を出す実装を残す
  if (useNativeForm && isLoginFlow(flow)) {
    return (
      <div className="login-container">
        <div className="login-card glass">
          <div className="login-header">
            <h1 className="gradient-text">Welcome back</h1>
            <p>Sign in to continue to Alt (Native Form)</p>
          </div>

          <form
            action={flow.ui.action}
            method={flow.ui.method || "post"}
            className="login-form"
          >
            <input type="hidden" name="csrf_token" value={csrf} />
            <input name="method" type="hidden" value="password" />

            <div className="form-group">
              <label htmlFor="identifier-native">Email</label>
              <input
                id="identifier-native"
                name="identifier"
                type="email"
                placeholder="Enter your email"
                className="form-input"
                required
              />
            </div>

            <div className="form-group">
              <label htmlFor="password-native">Password</label>
              <input
                id="password-native"
                type="password"
                name="password"
                placeholder="Enter your password"
                className="form-input"
                required
              />
            </div>

            <button
              type="submit"
              disabled={submitting}
              className="btn-accent login-button"
            >
              {submitting ? "Signing in..." : "Sign in (Native)"}
            </button>
          </form>

          <div className="login-footer">
            <button
              onClick={() => setUseNativeForm(false)}
              className="recovery-link unstyled-button"
            >
              ← Use SDK Form
            </button>
          </div>
        </div>

        <style jsx>{`
          .login-container {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: var(--space-4);
          }

          .login-card {
            width: 100%;
            max-width: 400px;
            padding: var(--space-8);
            margin: var(--space-4);
          }

          .login-header {
            text-align: center;
            margin-bottom: var(--space-8);
          }

          .login-header h1 {
            font-size: var(--text-3xl);
            margin-bottom: var(--space-2);
          }

          .login-header p {
            color: var(--text-muted);
            font-size: var(--text-sm);
          }

          .login-form {
            display: flex;
            flex-direction: column;
            gap: var(--space-6);
          }

          .form-group {
            display: flex;
            flex-direction: column;
            gap: var(--space-2);
          }

          .form-group label {
            font-size: var(--text-sm);
            font-weight: 600;
            color: var(--text-primary);
          }

          .form-input {
            padding: var(--space-4);
            border: 1px solid var(--surface-border);
            border-radius: var(--radius-lg);
            background: rgba(255, 255, 255, 0.05);
            color: var(--text-primary);
            font-size: var(--text-base);
            transition: all var(--transition-speed) ease;
            backdrop-filter: blur(10px);
          }

          .form-input:focus {
            outline: none;
            border-color: var(--alt-primary);
            box-shadow: 0 0 0 3px rgba(var(--alt-primary), 0.1);
            background: rgba(255, 255, 255, 0.1);
          }

          .form-input::placeholder {
            color: var(--text-muted);
            text-align: left;
          }

          .login-button {
            padding: var(--space-4) var(--space-6);
            font-size: var(--text-base);
            font-weight: 600;
            margin-top: var(--space-4);
            width: 100%;
          }

          .login-button:disabled {
            opacity: 0.6;
            cursor: not-allowed;
            transform: none;
          }

          .login-footer {
            text-align: center;
            margin-top: var(--space-6);
            padding-top: var(--space-6);
            border-top: 1px solid var(--surface-border);
          }

          .recovery-link {
            color: var(--alt-primary);
            font-size: var(--text-sm);
            text-decoration: none;
            cursor: pointer;
          }

          .recovery-link:hover {
            color: var(--alt-secondary);
            text-decoration: underline;
          }

          .unstyled-button {
            background: none;
            border: none;
          }

          @media (max-width: 480px) {
            .login-card {
              margin: 0;
              padding: var(--space-6);
            }

            .login-header h1 {
              font-size: var(--text-2xl);
            }
          }
        `}</style>
      </div>
    );
  }

  return (
    <div className="login-container">
      <div className="login-card glass">
        <div className="login-header">
          <h1 className="gradient-text">Welcome back</h1>
          <p>Sign in to continue to Alt</p>
        </div>

        <form onSubmit={onSubmit} className="login-form">
          <input type="hidden" name="csrf_token" value={csrf} />

          <div className="form-group">
            <label htmlFor="identifier">Email</label>
            <input
              id="identifier"
              name="identifier"
              type="email"
              placeholder="Enter your email"
              className="form-input"
              required
            />
          </div>

          <div className="form-group">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              name="password"
              type="password"
              placeholder="Enter your password"
              className="form-input"
              required
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="btn-accent login-button"
          >
            {submitting ? "Signing in..." : "Sign in"}
          </button>
        </form>

        <div className="login-footer">
          <div className="footer-links">
            <a href={recoveryUrl} className="footer-link">
              Forgot your password?
            </a>
            <span className="divider">•</span>
            <a href={registrationUrl} className="footer-link">
              Create a new account
            </a>
          </div>
        </div>

        {/* Debug toggle (only show in development) */}
        {process.env.NODE_ENV === "development" && (
          <button
            onClick={() => setUseNativeForm(true)}
            className="debug-toggle"
          >
            Use Native Form
          </button>
        )}
      </div>

      <style jsx>{`
        .login-container {
          min-height: 100vh;
          display: flex;
          align-items: center;
          justify-content: center;
          padding: var(--space-4);
        }

        .login-card {
          width: 100%;
          max-width: 400px;
          padding: var(--space-8);
          margin: var(--space-4);
        }

        .login-header {
          text-align: center;
          margin-bottom: var(--space-8);
        }

        .login-header h1 {
          font-size: var(--text-3xl);
          margin-bottom: var(--space-2);
        }

        .login-header p {
          color: var(--text-muted);
          font-size: var(--text-sm);
        }

        .login-form {
          display: flex;
          flex-direction: column;
          gap: var(--space-6);
        }

        .form-group {
          display: flex;
          flex-direction: column;
          gap: var(--space-2);
        }

        .form-group label {
          font-size: var(--text-sm);
          font-weight: 600;
          color: var(--text-primary);
        }

        .form-input {
          padding: var(--space-4);
          border: 1px solid var(--surface-border);
          border-radius: var(--radius-lg);
          background: rgba(255, 255, 255, 0.05);
          color: var(--text-primary);
          font-size: var(--text-base);
          transition: all var(--transition-speed) ease;
          backdrop-filter: blur(10px);
        }

        .form-input:focus {
          outline: none;
          border-color: var(--alt-primary);
          box-shadow: 0 0 0 3px rgba(var(--alt-primary), 0.1);
          background: rgba(255, 255, 255, 0.1);
        }

        .form-input::placeholder {
          color: var(--text-muted);
          text-align: left;
        }

        .login-button {
          padding: var(--space-4) var(--space-6);
          font-size: var(--text-base);
          font-weight: 600;
          margin-top: var(--space-4);
          width: 100%;
        }

        .login-button:disabled {
          opacity: 0.6;
          cursor: not-allowed;
          transform: none;
        }

        .login-footer {
          text-align: center;
          margin-top: var(--space-6);
          padding-top: var(--space-6);
          border-top: 1px solid var(--surface-border);
        }

        .footer-links {
          display: flex;
          gap: var(--space-2);
          justify-content: center;
          flex-wrap: wrap;
          font-size: var(--text-sm);
          color: var(--text-muted);
        }

        .footer-link {
          color: var(--alt-primary);
          text-decoration: none;
        }

        .footer-link:hover {
          color: var(--alt-secondary);
          text-decoration: underline;
        }

        .divider {
          color: var(--text-muted);
        }

        .debug-toggle {
          position: absolute;
          top: var(--space-2);
          right: var(--space-2);
          padding: var(--space-1) var(--space-2);
          font-size: var(--text-xs);
          background: var(--surface-bg);
          border: 1px solid var(--surface-border);
          border-radius: var(--radius-sm);
          color: var(--text-muted);
        }

        @media (max-width: 480px) {
          .login-card {
            margin: 0;
            padding: var(--space-6);
          }

          .login-header h1 {
            font-size: var(--text-2xl);
          }
        }
      `}</style>
    </div>
  );
}
