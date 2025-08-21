import { redirect } from 'next/navigation';
import { serverFetch } from '@/lib/server-fetch';

export default async function HomeLayout({ 
  children 
}: { 
  children: React.ReactNode 
}) {
  try {
    const data = await serverFetch<{valid: boolean}>('/v1/auth/validate');
    if (!data.valid) {
      // 認証が無効な場合、/home へのリダイレクト付きでログインページへ
      const appOrigin = process.env.NEXT_PUBLIC_APP_URL || 'https://curionoah.com';
      redirect(`/auth/login?return_to=${encodeURIComponent(`${appOrigin}/home`)}`);
    }
  } catch (error) {
    console.error('Home layout auth validation failed:', error);
    // エラーの場合もログインページへリダイレクト
    const appOrigin = process.env.NEXT_PUBLIC_APP_URL || 'https://curionoah.com';
    redirect(`/auth/login?return_to=${encodeURIComponent(`${appOrigin}/home`)}`);
  }
  
  return children;
}