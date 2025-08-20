import { headers } from 'next/headers';
import { redirect } from 'next/navigation';

export default async function MobileLayout({ 
  children 
}: { 
  children: React.ReactNode 
}) {
  const h = await headers();
  const cookie = h.get('cookie') ?? '';
  
  try {
    const res = await fetch(`${process.env.KRATOS_INTERNAL_URL}/sessions/whoami`, {
      headers: { cookie },
      cache: 'no-store',
    });
    
    if (res.status === 401) {
      const protocol = h.get('x-forwarded-proto') || 'https';
      const host = h.get('host') || 'curionoah.com';
      const path = h.get('x-invoke-path') || '/mobile/home';
      const returnTo = `${protocol}://${host}${path}`;
      redirect(`/auth/login?return_to=${encodeURIComponent(returnTo)}`);
    }
  } catch (error) {
    console.error('Session validation failed:', error);
    redirect('/auth/login?return_to=' + encodeURIComponent('https://curionoah.com/mobile/home'));
  }
  
  return children;
}