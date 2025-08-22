// src/app/auth/login/page.tsx
import { LoginForm } from './LoginForm';
import { kratos } from '@/lib/kratos';

export default async function LoginPage({ 
  searchParams 
}: { 
  searchParams: Promise<{ flow?: string }> 
}) {
  const params = await searchParams;
  
  if (!params.flow) {
    // 直叩きされたら login/browser へ送る（UI直アクセス防止）
    return (
      <script
        dangerouslySetInnerHTML={{
          __html: `
            const rt = encodeURIComponent(window.location.href);
            window.location.href = 'https://id.curionoah.com/self-service/login/browser?return_to=' + rt;
          `,
        }}
      />
    );
  }

  // Server側でFlow IDの有効性を事前チェック
  try {
    await kratos.getLoginFlow({ id: params.flow });
  } catch (error: any) {
    // Flow IDが無効な場合は新しいFlowへリダイレクト
    if (error?.response?.status === 410 || error?.status === 410) {
      return (
        <script
          dangerouslySetInnerHTML={{
            __html: `
              console.log('Invalid flow ID detected, redirecting to new flow');
              const rt = encodeURIComponent(window.location.href.split('?')[0]);
              window.location.replace('https://id.curionoah.com/self-service/login/browser?return_to=' + rt);
            `,
          }}
        />
      );
    }
  }
  
  return <LoginForm flowId={params.flow} />;
}