import { useState } from 'react';
import { UpdateLoginFlowBody } from '@ory/client';
import { kratos } from '@/lib/kratos';

export function useLoginSubmit(flowId: string) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (values: Record<string, string>) => {
    setSubmitting(true);
    setError(null);

    try {
      // password認証用の型定義
      const loginBody = {
        method: 'password' as const,
        identifier: values.identifier || values.email || '',
        password: values.password || '',
        csrf_token: values.csrf_token,
      } as UpdateLoginFlowBody;

      const response = await kratos.updateLoginFlow({
        flow: flowId,
        updateLoginFlowBody: loginBody,
      });

      // 成功時のリダイレクト処理 - ブラウザフローの場合は自動でリダイレクトされる
      if ('return_to' in response.data && typeof response.data.return_to === 'string') {
        window.location.href = response.data.return_to;
      } else {
        window.location.href = '/';
      }
    } catch (err: unknown) {
      console.error('Login failed:', err);
      
      if (err && typeof err === 'object' && 'response' in err) {
        const errorResponse = err.response as { data?: { ui?: { messages?: Array<{ type: string; text: string }> } } };
        if (errorResponse.data?.ui?.messages) {
          const errorMessages = errorResponse.data.ui.messages
            .filter(msg => msg.type === 'error')
            .map(msg => msg.text)
            .join(', ');
          setError(errorMessages);
        } else {
          setError('Login failed. Please check your credentials and try again.');
        }
      } else {
        setError('Login failed. Please check your credentials and try again.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return { submit, submitting, error };
}