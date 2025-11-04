"use client";

import type {
  ContinueWith,
  LoginFlow,
  RegistrationFlow,
  SuccessfulNativeLogin,
  SuccessfulNativeRegistration,
  UiNode,
  UiNodeInputAttributes,
  UpdateLoginFlowBody,
  UpdateRegistrationFlowBody,
} from "@ory/client";
import { useCallback, useEffect, useMemo, useState } from "react";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";
import { oryClient } from "./client";
import { formDataToSubmission, normalizeMethod } from "./form";

type FlowKind = "login" | "registration";

type AnyFlow = LoginFlow | RegistrationFlow;
type SuccessResult = SuccessfulNativeLogin | SuccessfulNativeRegistration;

interface UseOryFlowOptions {
  type: FlowKind;
  flowId?: string;
  returnTo?: string;
  onSuccess?: (result: SuccessResult) => void;
}

interface UseOryFlowResult {
  flow: AnyFlow | null;
  isLoading: boolean;
  isSubmitting: boolean;
  error: string | null;
  handleSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  refresh: () => Promise<void>;
}

const FLOW_BROWSER_ROUTES: Record<FlowKind, string> = {
  login: "/self-service/login/browser",
  registration: "/self-service/registration/browser",
};

const isFlow = (value: unknown): value is AnyFlow => {
  return !!value && typeof value === "object" && "id" in value && "ui" in value;
};

const extractRedirect = (result: SuccessResult | null | undefined): string | null => {
  if (!result) return null;
  const entries: ContinueWith[] | undefined = result.continue_with;
  if (!entries) return null;
  for (const entry of entries) {
    if (entry && typeof entry === "object" && "redirect_browser_to" in entry) {
      const target = (entry as { redirect_browser_to?: string }).redirect_browser_to;
      if (target) {
        return target;
      }
    }
  }
  return null;
};

const getNodeByName = (nodes: UiNode[], name: string) =>
  nodes.find((node) => {
    if (node.type !== "input") return false;
    const attrs = node.attributes as UiNodeInputAttributes;
    return attrs.name === name;
  });

export const useOryFlow = (options: UseOryFlowOptions): UseOryFlowResult => {
  const { type, flowId, returnTo, onSuccess } = options;

  const [flow, setFlow] = useState<AnyFlow | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  const redirectToBegin = useCallback(() => {
    if (typeof window === "undefined") return;
    const url = new URL(FLOW_BROWSER_ROUTES[type], KRATOS_PUBLIC_URL);
    if (returnTo) {
      url.searchParams.set("return_to", returnTo);
    }
    // Use replace instead of href to avoid adding to history stack
    window.location.replace(url.toString());
  }, [type, returnTo]);

  const loadFlow = useCallback(async () => {
    if (!flowId) {
      redirectToBegin();
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const fetcher =
        type === "login"
          ? oryClient.getLoginFlow({ id: flowId })
          : oryClient.getRegistrationFlow({ id: flowId });
      const { data } = await fetcher;
      setFlow(data as AnyFlow);
    } catch (err) {
      const status =
        (err as { response?: { status?: number }; status?: number }).response?.status ??
        (err as { status?: number }).status;

      // Handle flow expired, not found, or gone errors
      if (status === 403 || status === 404 || status === 410) {
        redirectToBegin();
        return;
      }

      // Handle CORS or network errors
      const errorMessage = (err as Error).message ?? "Flowを取得できませんでした";
      if (errorMessage.includes("CORS") || errorMessage.includes("Network")) {
        console.error("CORS or network error fetching flow:", err);
        setError("ネットワークエラーが発生しました。ページを再読み込みしてください。");
      } else {
        setError(errorMessage);
      }
    } finally {
      setIsLoading(false);
    }
  }, [flowId, redirectToBegin, type]);

  useEffect(() => {
    loadFlow();
  }, [loadFlow]);

  const handleSubmit = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();

      if (!flow) {
        setError("アクティブなフローがありません");
        return;
      }

      setIsSubmitting(true);
      setError(null);

      try {
        const submitEvent = event.nativeEvent as SubmitEvent;
        const submitter = submitEvent.submitter as HTMLInputElement | HTMLButtonElement | null;

        const formData = new FormData(event.currentTarget);
        if (submitter?.name) {
          formData.set(submitter.name, submitter.value);
        }

        const submission = formDataToSubmission(formData);

        if (!submission.method) {
          const methodNode = getNodeByName(flow.ui.nodes, "method");
          if (methodNode) {
            const methodAttrs = methodNode.attributes as UiNodeInputAttributes;
            const value =
              typeof methodAttrs.value === "string"
                ? methodAttrs.value
                : normalizeMethod(methodAttrs.value as string | undefined);
            if (value) {
              submission.method = value;
            }
          }
        } else {
          submission.method = normalizeMethod(submission.method);
        }

        let result:
          | LoginFlow
          | RegistrationFlow
          | SuccessfulNativeLogin
          | SuccessfulNativeRegistration;

        if (type === "login") {
          const { data } = await oryClient.updateLoginFlow({
            flow: flow.id,
            updateLoginFlowBody: submission as unknown as UpdateLoginFlowBody,
          });
          result = data as LoginFlow | SuccessfulNativeLogin;
        } else {
          const { data } = await oryClient.updateRegistrationFlow({
            flow: flow.id,
            updateRegistrationFlowBody: submission as unknown as UpdateRegistrationFlowBody,
          });
          result = data as RegistrationFlow | SuccessfulNativeRegistration;
        }

        // If result is a flow (validation errors, etc.), update UI with new flow
        if (isFlow(result)) {
          setFlow(result);
          return;
        }

        // Successful authentication/registration
        onSuccess?.(result as SuccessResult);

        // Follow Ory's redirect instructions
        const explicitRedirect = extractRedirect(result as SuccessResult);
        if (explicitRedirect) {
          // Use replace to avoid adding to history stack
          window.location.replace(explicitRedirect);
          return;
        }

        // Fallback redirect
        const fallback = returnTo ?? flow.return_to ?? "/";
        window.location.replace(fallback);
      } catch (err) {
        const response = (err as { response?: { status?: number; data?: unknown } }).response;
        const status = response?.status ?? (err as { status?: number }).status;

        // Handle validation errors (400/422) - update flow with error messages
        if ((status === 400 || status === 422) && response?.data && isFlow(response.data)) {
          setFlow(response.data as AnyFlow);
          return;
        }

        // Handle expired or invalid flows (403/404/410) - restart flow
        if (status === 403 || status === 404 || status === 410) {
          redirectToBegin();
          return;
        }

        // Handle CSRF token errors - restart flow
        // Ory Kratos automatically handles CSRF tokens, but in case of mismatch
        if (status === 400) {
          const errorMessage = (err as Error).message?.toLowerCase() || "";
          if (errorMessage.includes("csrf") || errorMessage.includes("token")) {
            redirectToBegin();
            return;
          }
        }

        // Generic error handling
        const errorMessage = (err as Error).message ?? "送信に失敗しました";
        setError(errorMessage);
      } finally {
        setIsSubmitting(false);
      }
    },
    [flow, onSuccess, redirectToBegin, returnTo, type]
  );

  return useMemo(
    () => ({
      flow,
      isLoading,
      isSubmitting,
      error,
      handleSubmit,
      refresh: loadFlow,
    }),
    [flow, isLoading, isSubmitting, error, handleSubmit, loadFlow]
  );
};
