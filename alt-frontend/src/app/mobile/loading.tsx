"use client";

export default function MobileLoading() {
  return (
    <div className="mobile-loading-container">
      <div className="mobile-loading-spinner" />
      <p className="mobile-loading-text">モバイルページを読み込み中...</p>
      <style jsx>{`
        .mobile-loading-container {
          padding: 24px;
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          min-height: 100vh;
          background-color: var(--app-bg);
        }
        .mobile-loading-spinner {
          width: 40px;
          height: 40px;
          border: 4px solid #e2e8f0;
          border-top: 4px solid #3182ce;
          border-radius: 50%;
          margin-bottom: 16px;
          animation: spin 1s linear infinite;
        }
        .mobile-loading-text {
          color: #718096;
        }
        @keyframes spin {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
}
