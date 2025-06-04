// app/layout.tsx
import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

// Chakra UI 関連 import
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";

// カスタム Provider をインポート
import { Provider as CustomColorModeProvider } from "@/components/ui/provider";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});
const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Alt",
  description: "Alt: A simple RSS reader",
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={`${geistSans.variable} ${geistMono.variable}`}>
        <ChakraProvider value={defaultSystem}>
          <CustomColorModeProvider>{children}</CustomColorModeProvider>
        </ChakraProvider>
      </body>
    </html>
  );
}
