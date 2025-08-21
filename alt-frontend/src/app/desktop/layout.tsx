export default function DesktopLayout({ 
  children 
}: { 
  children: React.ReactNode 
}) {
  // 認証チェックは Middleware で完了済み
  // Layout は純粋に UI のレイアウトのみを担当
  return (
    <div className="desktop-layout">
      {children}
    </div>
  );
}