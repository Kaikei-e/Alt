"use client";

import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ThemeProvider as NextThemesProvider } from "next-themes";
import { useEffect } from "react";
import { AuthProvider } from "@/contexts/auth-context";
import { ThemeProvider } from "@/providers/ThemeProvider";
import { setupErrorHandling } from "@/utils/reload-guard";

export function Providers({ children, nonce }: { children: React.ReactNode; nonce: string }) {
  useEffect(() => {
    setupErrorHandling();
  }, []);

  return (
    <ChakraProvider value={defaultSystem}>
      <NextThemesProvider
        attribute="data-style"
        defaultTheme="alt-paper"
        themes={["vaporwave", "alt-paper"]}
        nonce={nonce}
      >
        <ThemeProvider>
          <AuthProvider>{children}</AuthProvider>
        </ThemeProvider>
      </NextThemesProvider>
    </ChakraProvider>
  );
}
