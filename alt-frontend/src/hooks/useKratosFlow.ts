import type { LoginFlow } from "@ory/client";
import { useEffect, useState } from "react";
import { kratos } from "@/lib/kratos";
import {
  getFlowErrorMessage,
  handleFlowExpiredError,
  isFlowExpiredError,
} from "@/lib/kratos-errors";

export function useKratosFlow(flowId: string) {
  const [flow, setFlow] = useState<LoginFlow | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchFlow = async () => {
      try {
        setLoading(true);
        const response = await kratos.getLoginFlow({ id: flowId });
        setFlow(response.data);
        setError(null);
      } catch (err) {
        console.error("Failed to fetch login flow:", err);

        if (isFlowExpiredError(err)) {
          setError(getFlowErrorMessage(err));
          // 410エラーの場合は自動的にリダイレクト
          handleFlowExpiredError(err);
          return;
        }

        setError(getFlowErrorMessage(err));
      } finally {
        setLoading(false);
      }
    };

    if (flowId) {
      fetchFlow();
    }
  }, [flowId]);

  return { flow, loading, error };
}
