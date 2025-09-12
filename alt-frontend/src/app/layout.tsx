// app/layout.tsx
import type { Metadata } from "next";
import { Providers } from "./providers";
import "./globals.css";
import { headers, cookies } from "next/headers";

// REPORT.md恒久対応: App Router 動的ルーティング安定化設定
// Next.js App Router 404バグ対策: 明示的ISR無効化でルーティング安定化
export const dynamic = "force-dynamic";
export const revalidate = 0;

export const metadata: Metadata = {
  title: "Alt - AI-powered RSS reader",
  description: "An AI-powered RSS reader with modern aesthetics.",
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "any" },
      { url: "/icon.svg", type: "image/svg+xml" },
    ],
    apple: "/apple-touch-icon.png",
  },
  // manifest: "/manifest.json", // Disabled - not using PWA functionality
  other: {
    "theme-color": "#1a1a2e",
  },
};

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const headersList = await headers();
  const nonce = headersList.get("x-nonce") || "";

  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Providers nonce={nonce}>{children}</Providers>
      </body>
    </html>
  );
}
