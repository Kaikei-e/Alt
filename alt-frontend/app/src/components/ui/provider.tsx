"use client";

import { ChakraProvider } from "@chakra-ui/react";
import { ColorModeProvider, type ColorModeProviderProps } from "./color-mode";
import { altSystem } from "@/theme/alt-theme";

export function Provider(props: ColorModeProviderProps) {
  return (
    <ChakraProvider value={altSystem}>
      <ColorModeProvider {...props} />
    </ChakraProvider>
  );
}
