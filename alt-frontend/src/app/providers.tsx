"use client";

import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ThemeProvider } from "@/providers/ThemeProvider";
import { ThemeProvider as NextThemesProvider } from "next-themes";
import { AuthProvider } from "@/contexts/auth-context";
import { useEffect } from "react";
import { setupErrorHandling } from "@/utils/reload-guard";

export function Providers({
  children,
  nonce,
}: {
  children: React.ReactNode;
  nonce: string;
}) {
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
          <AuthProvider>
            {children}
          </AuthProvider>
        </ThemeProvider>
      </NextThemesProvider>
    </ChakraProvider>
  );
}
