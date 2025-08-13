'use client'

import { useState } from 'react'
import { feedsApi } from '@/lib/api'

export default function TestAuthPage() {
  const [result, setResult] = useState<string>('')
  const [isLoading, setIsLoading] = useState(false)

  const testApiCall = async () => {
    setIsLoading(true)
    setResult('API呼び出し中...')
    
    try {
      // This should trigger the 401 interceptor
      const data = await feedsApi.getFeedsWithCursor()
      setResult(`成功: ${JSON.stringify(data, null, 2)}`)
    } catch (error) {
      setResult(`エラー: ${error instanceof Error ? error.message : 'Unknown error'}`)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-md mx-auto">
        <div className="text-center">
          <h1 className="text-3xl font-extrabold text-gray-900 mb-8">
            🧪 401インターセプターテスト
          </h1>
          
          <div className="space-y-4">
            <button
              onClick={testApiCall}
              disabled={isLoading}
              className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
            >
              {isLoading ? '呼び出し中...' : 'APIを呼び出す（401エラー期待）'}
            </button>
            
            <div className="mt-6">
              <h3 className="text-lg font-medium text-gray-900 mb-2">期待される動作:</h3>
              <ol className="text-left text-sm text-gray-600 space-y-1">
                <li>1. ボタンをクリック</li>
                <li>2. alt-backendから401レスポンス</li>
                <li>3. 自動的にログインページへリダイレクト</li>
                <li>4. returnURLが正しく設定される</li>
              </ol>
            </div>
            
            {result && (
              <div className="mt-6 p-4 bg-gray-100 rounded-md">
                <h3 className="text-sm font-medium text-gray-900 mb-2">結果:</h3>
                <pre className="text-xs text-gray-600 whitespace-pre-wrap">
                  {result}
                </pre>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}