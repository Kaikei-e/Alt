"use client";
import { useEffect } from "react";

export default function DesktopError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Log detailed error information for debugging
    console.error("[DesktopError]", {
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
    <div className="desktop-error-container">
      <h2 className="desktop-error-title">デスクトップエラー</h2>
      <p className="desktop-error-message">
        デスクトップページでエラーが発生しました。
      </p>
      <button onClick={handleReset} className="desktop-error-button">
        再試行
      </button>
      {process.env.NODE_ENV === "development" && (
        <details className="desktop-error-details">
          <summary className="desktop-error-summary">
            エラー詳細 (開発環境のみ)
          </summary>
          <pre className="desktop-error-pre">{error.stack}</pre>
        </details>
      )}
      <style jsx>{`
        .desktop-error-container {
          padding: 24px;
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          min-height: 100vh;
          background-color: var(--app-bg);
        }
        .desktop-error-title {
          margin-bottom: 16px;
          color: #e53e3e;
          font-size: 1.5rem;
        }
        .desktop-error-message {
          margin-bottom: 24px;
          color: #718096;
        }
        .desktop-error-button {
          background-color: #3182ce;
          color: white;
          border: none;
          padding: 12px 24px;
          border-radius: 6px;
          cursor: pointer;
          font-size: 16px;
        }
        .desktop-error-details {
          margin-top: 2rem;
          text-align: left;
          width: 100%;
          max-width: 600px;
        }
        .desktop-error-summary {
          cursor: pointer;
          margin-bottom: 1rem;
        }
        .desktop-error-pre {
          background: #f7fafc;
          padding: 1rem;
          overflow: auto;
          font-size: 12px;
          border-radius: 4px;
          border: 1px solid #e2e8f0;
        }
      `}</style>
    </div>
  );
}
