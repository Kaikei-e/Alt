"use client";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  console.error(error); // k8sログに流す

  const handleReset = () => {
    reset();
  };

  return (
    <div className="error-container">
      <h2 className="error-title">問題が発生しました</h2>
      <p className="error-message">
        アプリケーションでエラーが発生しました。もう一度お試しください。
      </p>
      <button
        onClick={handleReset}
        className="error-button"
      >
        もう一度試す
      </button>
      <style jsx>{`
        .error-container {
          padding: 24px;
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          min-height: 50vh;
        }
        .error-title {
          margin-bottom: 16px;
          color: #e53e3e;
        }
        .error-message {
          margin-bottom: 24px;
          color: #718096;
        }
        .error-button {
          background-color: #3182ce;
          color: white;
          border: none;
          padding: 8px 16px;
          border-radius: 4px;
          cursor: pointer;
        }
      `}</style>
    </div>
  );
}
