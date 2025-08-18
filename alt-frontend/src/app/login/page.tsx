export const dynamic = 'force-dynamic'
export const revalidate = 0

import { redirect } from 'next/navigation'
import LoginClient from './login-client'

export default async function LoginPageServer({
  searchParams,
}: { searchParams: Promise<{ flow?: string; refresh?: string; return_to?: string }> }) {
  const params = await searchParams
  if (!params.flow) {
    const q = params.refresh === 'true' ? '?refresh=true' : ''
    redirect(`https://id.curionoah.com/self-service/login/browser${q}`)
  }
  return <LoginClient flowId={params.flow!} returnUrl={params.return_to || '/'} />
}