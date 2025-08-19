'use client'
import { useEffect } from 'react'

export default function MobileError({ 
  error, 
  reset 
}: { 
  error: Error & { digest?: string }
  reset: () => void 
}) {
  useEffect(() => {
    // Log detailed error information for debugging
    console.error('[MobileError]', {
      message: error.message,
      stack: error.stack,
      digest: error.digest,
      name: error.name,
      timestamp: new Date().toISOString()
    })
  }, [error])

  return (
    <div style={{ 
      padding: 16, 
      display: 'flex', 
      flexDirection: 'column', 
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
      backgroundColor: 'var(--app-bg)',
      maxWidth: '400px',
      margin: '0 auto'
    }}>
      <h2 style={{ marginBottom: 16, color: '#e53e3e', fontSize: '1.25rem' }}>
        モバイルエラー
      </h2>
      <p style={{ marginBottom: 24, color: '#718096', textAlign: 'center' }}>
        モバイルページでエラーが発生しました。
      </p>
      <button 
        onClick={() => reset()}
        style={{
          backgroundColor: '#3182ce',
          color: 'white',
          border: 'none',
          padding: '12px 24px',
          borderRadius: '6px',
          cursor: 'pointer',
          fontSize: '16px',
          width: '100%'
        }}
      >
        再試行
      </button>
      {process.env.NODE_ENV === 'development' && (
        <details style={{ marginTop: '2rem', textAlign: 'left', width: '100%' }}>
          <summary style={{ cursor: 'pointer', marginBottom: '1rem' }}>エラー詳細 (開発環境のみ)</summary>
          <pre style={{ 
            background: '#f7fafc', 
            padding: '1rem', 
            overflow: 'auto',
            fontSize: '10px',
            borderRadius: '4px',
            border: '1px solid #e2e8f0'
          }}>
            {error.stack}
          </pre>
        </details>
      )}
    </div>
  )
}