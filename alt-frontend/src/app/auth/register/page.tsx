import { redirect } from 'next/navigation'
import { Suspense } from 'react'
import RegisterClient from './register-client'

export default async function RegisterPageServer({
  searchParams,
}: { searchParams: Promise<{ flow?: string; return_to?: string }> }) {
  const params = await searchParams
  if (!params.flow) {
    redirect('https://id.curionoah.com/self-service/registration/browser')
  }
  return <RegisterClient flowId={params.flow!} returnUrl={params.return_to || '/'} />
}

