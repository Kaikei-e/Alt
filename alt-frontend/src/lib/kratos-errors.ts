export interface KratosErrorResponse {
  error?: {
    id?: string;
    code?: number;
    status?: string;
    reason?: string;
    message?: string;
    details?: {
      redirect_to?: string;
      return_to?: string;
    };
  };
}

export function isFlowExpiredError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;

  // @ory/client の Error オブジェクトを想定
  const kratosError = error as any;
  return (
    kratosError.response?.status === 410 ||
    kratosError.response?.data?.error?.id === "self_service_flow_expired" ||
    kratosError.status === 410
  );
}

export function extractRedirectUrl(error: unknown): string | null {
  if (!error || typeof error !== "object") return null;

  const kratosError = error as any;
  return kratosError.response?.data?.error?.details?.redirect_to || null;
}

export function handleFlowExpiredError(error: unknown): void {
  if (!isFlowExpiredError(error)) return;

  const redirectUrl = extractRedirectUrl(error);
  if (redirectUrl) {
    window.location.replace(redirectUrl);
  } else {
    // Fallback: redirect to same-origin /ory login browser endpoint
    const currentUrl = window.location.href;
    const returnTo = encodeURIComponent(currentUrl.split("?")[0]);
    window.location.replace(
      `/ory/self-service/login/browser?return_to=${returnTo}`,
    );
  }
}

export function getFlowErrorMessage(error: unknown): string {
  if (isFlowExpiredError(error)) {
    return "セッションが期限切れです。新しいログインフローを開始しています...";
  }

  if (error && typeof error === "object") {
    const kratosError = error as any;
    return (
      kratosError.response?.data?.error?.message ||
      "Error loading login form. Please try again."
    );
  }

  return "Error loading login form. Please try again.";
}
