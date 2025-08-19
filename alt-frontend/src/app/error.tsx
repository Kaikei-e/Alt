'use client'

export default function Error({ 
  error, 
  reset 
}: { 
  error: Error & { digest?: string }
  reset: () => void 
}) {
  console.error(error) // k8sログに流す
  
  return (
    <div style={{ 
      padding: 24, 
      display: 'flex', 
      flexDirection: 'column', 
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '50vh'
    }}>
      <h2 style={{ marginBottom: 16, color: '#e53e3e' }}>
        問題が発生しました
      </h2>
      <p style={{ marginBottom: 24, color: '#718096' }}>
        アプリケーションでエラーが発生しました。もう一度お試しください。
      </p>
      <button 
        onClick={() => reset()}
        style={{
          backgroundColor: '#3182ce',
          color: 'white',
          border: 'none',
          padding: '8px 16px',
          borderRadius: '4px',
          cursor: 'pointer'
        }}
      >
        もう一度試す
      </button>
    </div>
  )
}