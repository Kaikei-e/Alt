'use client';
import React, { useState } from 'react';
import { UiNode, Configuration, FrontendApi, UpdateLoginFlowBody } from '@ory/client';

interface LoginFormProps {
  flow: any;
}

const frontend = new FrontendApi(
  new Configuration({ basePath: process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL })
)

export default function LoginForm({ flow }: LoginFormProps) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);

    const formData = new FormData(event.currentTarget);
    const body: UpdateLoginFlowBody = {
      method: 'password',
      identifier: String(formData.get('identifier') || ''),
      password: String(formData.get('password') || ''),
      csrf_token: String(formData.get('csrf_token') || ''),
    };

    try {
      await frontend.updateLoginFlow({ flow: flow.id, updateLoginFlowBody: body });
      // 成功時は Kratos が Set-Cookie + リダイレクト/JSONを返す
      // ブラウザは `return_to` へ遷移
      window.location.replace(process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT || 'https://curionoah.com/desktop/home');
    } catch (e: any) {
      console.error('updateLoginFlow failed', e);
      setError('Login failed. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

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