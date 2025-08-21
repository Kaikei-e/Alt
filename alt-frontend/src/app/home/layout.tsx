import { headers } from 'next/headers';
import { redirect } from 'next/navigation';

export default async function HomeLayout({ 
  children 
}: { 
  children: React.ReactNode 
}) {
  const h = await headers();
  const cookie = h.get('cookie') ?? '';
  
  try {
    // 正しい認証検証エンドポイントを使用
    const res = await fetch(`${process.env.AUTH_URL}/v1/auth/validate`, {
      headers: { cookie },
      cache: 'no-store',
    });
    
    if (res.status === 200) {
      const data = await res.json();
      if (!data.valid) {
        // 認証が無効な場合、/home へのリダイレクト付きでログインページへ
        const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || 'https://curionoah.com';
        redirect(`/auth/login?return_to=${encodeURIComponent(`${appOrigin}/home`)}`);
      }
    } else if (res.status === 401) {
      // 401 Unauthorized の場合、ログインページへリダイレクト
      const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || 'https://curionoah.com';
      redirect(`/auth/login?return_to=${encodeURIComponent(`${appOrigin}/home`)}`);
    }
  } catch (error) {
    console.error('Home layout auth validation failed:', error);
    // エラーの場合もログインページへリダイレクト
    const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || 'https://curionoah.com';
    redirect(`/auth/login?return_to=${encodeURIComponent(`${appOrigin}/home`)}`);
  }
  
  return children;
}