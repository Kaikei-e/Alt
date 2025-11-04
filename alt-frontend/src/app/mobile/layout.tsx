export default function MobileLayout({ children }: { children: React.ReactNode }) {
  // 認証チェックは Middleware で完了済み
  // Layout は純粋に UI のレイアウトのみを担当
  return <div className="mobile-layout">{children}</div>;
}
