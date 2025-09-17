// app/auth/error/page.tsx
export default function Page() {
  return (
    <main className="p-6">
      <h1>Authentication Error</h1>
      <p>ログインに失敗しました。もう一度お試しください。</p>
      <a href="/auth/login">ログインし直す</a>
    </main>
  );
}
