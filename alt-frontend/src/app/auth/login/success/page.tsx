import { redirect } from 'next/navigation'

export default async function Page() {
  if (process.env.NEXT_PUBLIC_FEATURE_COOKIE_MIGRATE === '1') {
    redirect('/auth/cookie-migrate?next=/')   // nextは任意に
  }
  redirect('/')
}