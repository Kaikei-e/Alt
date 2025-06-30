"use client";

import { ChakraProvider } from "@chakra-ui/react";
import { ColorModeProvider, type ColorModeProviderProps } from "./color-mode";
import { altSystem } from "@/theme/alt-theme";
import { ThemeProvider } from "@/providers/ThemeProvider";

export function Provider(props: ColorModeProviderProps) {
  return (
    <ChakraProvider value={altSystem}>
      <ThemeProvider>
        <ColorModeProvider {...props} />
      </ThemeProvider>
    </ChakraProvider>
  );
}
