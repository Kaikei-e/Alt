"use client";

export default function DesktopLoading() {
  return (
    <div className="desktop-loading-container">
      <div className="desktop-loading-spinner" />
      <p className="desktop-loading-text">デスクトップページを読み込み中...</p>
      <style jsx>{`
        .desktop-loading-container {
          padding: 24px;
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          min-height: 100vh;
          background-color: var(--app-bg);
        }
        .desktop-loading-spinner {
          width: 40px;
          height: 40px;
          border: 4px solid #e2e8f0;
          border-top: 4px solid #3182ce;
          border-radius: 50%;
          margin-bottom: 16px;
          animation: spin 1s linear infinite;
        }
        .desktop-loading-text {
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
