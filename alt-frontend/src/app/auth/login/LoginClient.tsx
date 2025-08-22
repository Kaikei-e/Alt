'use client'
import { useEffect, useState } from 'react'
import { Configuration, FrontendApi, UpdateLoginFlowBody } from '@ory/client'

const frontend = new FrontendApi(
  new Configuration({ basePath: process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL })
)

export default function LoginClient() {
  const [flowId, setFlowId] = useState<string | null>(null)
  const [flow, setFlow] = useState<any>(null)
  const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN!
  const kratos = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL!

  // 1) flowID 取得 or 新規作成
  useEffect(() => {
    const url = new URL(window.location.href)
    const f = url.searchParams.get('flow')
    if (!f) {
      const returnTo = encodeURIComponent(window.location.href)
      window.location.replace(
        `${kratos}/self-service/login/browser?return_to=${returnTo}`
      )
      return
    }
    setFlowId(f)
  }, [])

  // 2) flow 読み込み（410はリダイレクトで復旧）
  useEffect(() => {
    if (!flowId) return
    ;(async () => {
      try {
        const { data } = await frontend.getLoginFlow({ id: flowId })
        setFlow(data)
      } catch (e: any) {
        const status = e?.response?.status ?? e?.status
        const errId = e?.response?.data?.error?.id
        if (status === 410 || errId === 'self_service_flow_expired') {
          const rt = encodeURIComponent(window.location.href.split('?')[0])
          window.location.replace(
            `${kratos}/self-service/login/browser?return_to=${rt}`
          )
          return
        }
        // 既にセッションあり（session_already_available）なら既定へ
        if (errId === 'session_already_available') {
          window.location.replace(
            process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT!
          )
          return
        }
        console.error('getLoginFlow failed', e)
      }
    })()
  }, [flowId])

  // 3) Submit（password例）
  async function onSubmit(ev: React.FormEvent<HTMLFormElement>) {
    ev.preventDefault()
    const form = new FormData(ev.currentTarget)
    const body: UpdateLoginFlowBody = {
      method: 'password',
      identifier: String(form.get('identifier') || ''),
      password: String(form.get('password') || ''),
      csrf_token: String(form.get('csrf_token') || ''),
    }
    try {
      await frontend.updateLoginFlow({ flow: flowId!, updateLoginFlowBody: body })
      // 成功時は Kratos が Set-Cookie + リダイレクト/JSONを返す
      // ブラウザは `return_to` へ遷移
      // 念のためwhoamiで確認してから移動したければここでチェックしてもよい
      window.location.replace(process.env.NEXT_PUBLIC_RETURN_TO_DEFAULT!)
    } catch (e) {
      console.error('updateLoginFlow failed', e)
      // TODO: エラーUI
    }
  }

  if (!flow) return <div>Loading…</div>

  // 最小UI（nodesからCSRF/identifier/passwordを拾う前提の簡易例）
  const nodes = flow.ui?.nodes ?? []
  const csrf = nodes.find((n: any) => n.attributes?.name === 'csrf_token')?.attributes?.value ?? ''

  return (
    <form onSubmit={onSubmit}>
      <input type="hidden" name="csrf_token" value={csrf} />
      <input name="identifier" placeholder="email" />
      <input name="password" type="password" placeholder="password" />
      <button type="submit">Sign in</button>
    </form>
  )
}