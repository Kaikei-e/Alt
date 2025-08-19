'use client'
import { useEffect } from 'react'

export default function GlobalError({ 
  error, 
  reset 
}: { 
  error: Error & { digest?: string }
  reset: () => void 
}) {
  useEffect(() => {
    console.error(error) // k8s ログにフルスタック
  }, [error])

  return (
    <html>
      <body>
        <div style={{ padding: '2rem', fontFamily: 'system-ui' }}>
          <h1>エラーが発生しました。</h1>
          <p>再試行してください。</p>
          <button onClick={reset} style={{ padding: '0.5rem 1rem', margin: '1rem 0' }}>
            再試行
          </button>
          {process.env.NODE_ENV === 'development' && (
            <details style={{ marginTop: '1rem' }}>
              <summary>エラー詳細 (開発環境のみ)</summary>
              <pre style={{ background: '#f5f5f5', padding: '1rem', overflow: 'auto' }}>
                {error.stack}
              </pre>
            </details>
          )}
        </div>
      </body>
    </html>
  )
}