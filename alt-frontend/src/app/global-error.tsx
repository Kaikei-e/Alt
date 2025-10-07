"use client";
import { useEffect } from "react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error); // k8s ログにフルスタック
  }, [error]);

  const handleReset = () => {
    reset();
  };

  return (
    <html>
      <body>
        <div className="global-error-container">
          <h1>エラーが発生しました。</h1>
          <p>再試行してください。</p>
          <button onClick={handleReset} className="global-error-button">
            再試行
          </button>
          {process.env.NODE_ENV === "development" && (
            <details className="global-error-details">
              <summary>エラー詳細 (開発環境のみ)</summary>
              <pre className="global-error-pre">{error.stack}</pre>
            </details>
          )}
        </div>
        <style jsx>{`
          .global-error-container {
            padding: 2rem;
            font-family: system-ui;
          }
          .global-error-button {
            padding: 0.5rem 1rem;
            margin: 1rem 0;
          }
          .global-error-details {
            margin-top: 1rem;
          }
          .global-error-pre {
            background: #f5f5f5;
            padding: 1rem;
            overflow: auto;
          }
        `}</style>
      </body>
    </html>
  );
}
