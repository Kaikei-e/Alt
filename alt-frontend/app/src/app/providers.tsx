"use client";

import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ThemeProvider } from "@/providers/ThemeProvider";
import { ThemeProvider as NextThemesProvider } from "next-themes";

export function Providers({
  children,
  nonce,
}: {
  children: React.ReactNode;
  nonce: string;
}) {
  return (
    <ChakraProvider value={defaultSystem}>
      <NextThemesProvider
        attribute="data-style"
        defaultTheme="liquid-beige"
        themes={["vaporwave", "liquid-beige"]}
        nonce={nonce}
      >
        <ThemeProvider>{children}</ThemeProvider>
      </NextThemesProvider>
    </ChakraProvider>
  );
}
