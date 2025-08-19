import { redirect } from 'next/navigation'

export default function LegacyRegisterRedirect() {
  // 308 Permanent Redirect to unified auth path
  redirect('/auth/register')
}