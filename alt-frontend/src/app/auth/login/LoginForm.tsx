'use client';
import React from 'react';
import { UiNode } from '@ory/client';
import { useKratosFlow } from '@/hooks/useKratosFlow';
import { useLoginSubmit } from '@/hooks/useLoginSubmit';
import { isFlowExpiredError } from '@/lib/kratos-errors';

interface LoginFormProps {
  flowId: string;
}

export function LoginForm({ flowId }: LoginFormProps) {
  const { flow, loading, error: flowError } = useKratosFlow(flowId);
  const { submit, submitting, error: submitError } = useLoginSubmit(flowId);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    
    if (!flow) return;

    const formData = new FormData(event.currentTarget);
    const values: Record<string, string> = {};
    
    formData.forEach((value, key) => {
      if (typeof value === 'string') {
        values[key] = value;
      }
    });

    await submit(values);
  };

  const error = flowError || submitError;

  if (loading) {
    return (
      <div className="flex justify-center items-center min-h-64">
        <div className="text-lg">Loading login form...</div>
      </div>
    );
  }

  if (error && !flow) {
    const isExpired = error.includes('セッションが期限切れです');
    
    return (
      <div className={`border rounded-md p-4 ${isExpired ? 'bg-yellow-50 border-yellow-200' : 'bg-red-50 border-red-200'}`}>
        <div className={isExpired ? 'text-yellow-800' : 'text-red-800'}>{error}</div>
        {isExpired && (
          <div className="mt-3">
            <button
              onClick={() => {
                const currentUrl = window.location.href;
                const returnTo = encodeURIComponent(currentUrl.split('?')[0]);
                const idpOrigin = process.env.NEXT_PUBLIC_IDP_ORIGIN || 'https://id.curionoah.com';
                window.location.replace(`${idpOrigin}/self-service/login/browser?return_to=${returnTo}`);
              }}
              className="bg-blue-600 text-white py-2 px-4 rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
            >
              新しいログインフローを開始
            </button>
          </div>
        )}
      </div>
    );
  }

  if (!flow) {
    return (
      <div className="bg-yellow-50 border border-yellow-200 rounded-md p-4">
        <div className="text-yellow-800">No login flow available.</div>
      </div>
    );
  }

  return (
    <div className="max-w-md mx-auto">
      <h1 className="text-2xl font-bold mb-6 text-center">Sign In</h1>
      
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-md p-4 mb-4">
          <div className="text-red-800">{error}</div>
        </div>
      )}

      <form 
        action={flow.ui.action} 
        method={flow.ui.method} 
        onSubmit={handleSubmit}
        className="space-y-4"
      >
        {flow.ui.nodes.map((node: UiNode, index: number) => {
          if (node.type === 'input') {
            const attrs = node.attributes as {
              name: string;
              type: string;
              required?: boolean;
              value?: string;
            };
            
            if (attrs.type === 'hidden') {
              return (
                <input
                  key={index}
                  type="hidden"
                  name={attrs.name}
                  value={attrs.value || ''}
                />
              );
            }

            const isEmail = attrs.name === 'identifier' || attrs.type === 'email';
            const isPassword = attrs.type === 'password';

            return (
              <div key={index}>
                <label 
                  htmlFor={attrs.name}
                  className="block text-sm font-medium text-gray-700 mb-1"
                >
                  {isEmail ? 'Email' : isPassword ? 'Password' : attrs.name}
                  {attrs.required && <span className="text-red-500 ml-1">*</span>}
                </label>
                <input
                  id={attrs.name}
                  name={attrs.name}
                  type={attrs.type}
                  required={attrs.required}
                  autoComplete={isEmail ? 'email' : isPassword ? 'current-password' : undefined}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                />
                {node.messages?.map((message, msgIndex) => (
                  <div 
                    key={msgIndex}
                    className={`text-sm mt-1 ${
                      message.type === 'error' ? 'text-red-600' : 'text-gray-600'
                    }`}
                  >
                    {message.text}
                  </div>
                ))}
              </div>
            );
          }

          if (node.type === 'script') {
            return null; // Skip scripts for security
          }

          return null;
        })}

        <button
          type="submit"
          disabled={submitting}
          className="w-full bg-blue-600 text-white py-2 px-4 rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {submitting ? 'Signing In...' : 'Sign In'}
        </button>
      </form>

    </div>
  );
}