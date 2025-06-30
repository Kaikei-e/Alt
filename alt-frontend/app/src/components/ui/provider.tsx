"use client";

import { ChakraProvider } from "@chakra-ui/react";
import { ColorModeProvider } from "./color-mode";
import { altSystem } from "@/theme/alt-theme";
import { ThemeProvider } from "@/providers/ThemeProvider";
import React from "react";

export function Provider({ children }: React.PropsWithChildren) {
  return (
    <ChakraProvider value={altSystem}>
      <ThemeProvider>
        <ColorModeProvider>{children}</ColorModeProvider>
      </ThemeProvider>
    </ChakraProvider>
  );
}
