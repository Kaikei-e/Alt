"use client";
import { useEffect } from "react";

export default function MobileError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Log detailed error information for debugging
    console.error("[MobileError]", {
      message: error.message,
      stack: error.stack,
      digest: error.digest,
      name: error.name,
      timestamp: new Date().toISOString(),
    });
  }, [error]);

  const handleReset = () => {
    reset();
  };

  return (
    <div className="mobile-error-container">
      <h2 className="mobile-error-title">モバイルエラー</h2>
      <p className="mobile-error-message">モバイルページでエラーが発生しました。</p>
      <button onClick={handleReset} className="mobile-error-button">
        再試行
      </button>
      {process.env.NODE_ENV === "development" && (
        <details className="mobile-error-details">
          <summary className="mobile-error-summary">エラー詳細 (開発環境のみ)</summary>
          <pre className="mobile-error-pre">{error.stack}</pre>
        </details>
      )}
      <style jsx>{`
        .mobile-error-container {
          padding: 16px;
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          min-height: 100dvh;
          background-color: var(--app-bg);
          max-width: 400px;
          margin: 0 auto;
        }
        .mobile-error-title {
          margin-bottom: 16px;
          color: #e53e3e;
          font-size: 1.25rem;
        }
        .mobile-error-message {
          margin-bottom: 24px;
          color: #718096;
          text-align: center;
        }
        .mobile-error-button {
          background-color: #3182ce;
          color: white;
          border: none;
          padding: 12px 24px;
          border-radius: 6px;
          cursor: pointer;
          font-size: 16px;
          width: 100%;
        }
        .mobile-error-details {
          margin-top: 2rem;
          text-align: left;
          width: 100%;
        }
        .mobile-error-summary {
          cursor: pointer;
          margin-bottom: 1rem;
        }
        .mobile-error-pre {
          background: #f7fafc;
          padding: 1rem;
          overflow: auto;
          font-size: 10px;
          border-radius: 4px;
          border: 1px solid #e2e8f0;
        }
      `}</style>
    </div>
  );
}
