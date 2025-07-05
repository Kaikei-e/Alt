"use client";

import type { AppProps } from 'next/app';
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ThemeProvider } from "@/providers/ThemeProvider";
import { ThemeProvider as NextThemesProvider } from "next-themes";
import "@/app/globals.css";

export default function App({ Component, pageProps }: AppProps) {
  return (
    <ChakraProvider value={defaultSystem}>
      <NextThemesProvider
        attribute="data-style"
        defaultTheme="liquid-beige"
        themes={["vaporwave", "liquid-beige"]}
      >
        <ThemeProvider>
          <Component {...pageProps} />
        </ThemeProvider>
      </NextThemesProvider>
    </ChakraProvider>
  );
}