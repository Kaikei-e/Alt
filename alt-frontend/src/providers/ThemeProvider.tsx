"use client";

import { useTheme as useNextTheme } from "next-themes";
import type React from "react";
import { useEffect } from "react";
import { ThemeContext } from "@/contexts/ThemeContext";
import { THEME_CONFIGS, type Theme } from "@/types/theme";

export const ThemeProvider = ({ children }: { children: React.ReactNode }) => {
  const { theme, setTheme } = useNextTheme();

  // Ensure the theme is valid, fallback to liquid-beige
  const isValidTheme = (t: string | undefined): t is Theme =>
    t === "vaporwave" || t === "alt-paper";
  const currentTheme = isValidTheme(theme) ? theme : "alt-paper";

  const contextValue = {
    currentTheme,
    toggleTheme: () => {
      const nextTheme =
        currentTheme === "alt-paper" ? "vaporwave" : "alt-paper";
      setTheme(nextTheme);
    },
    setTheme,
    themeConfig: THEME_CONFIGS[currentTheme],
  };

  useEffect(() => {
    if (typeof document !== "undefined")
      document.body.setAttribute("data-style", currentTheme);
  }, [currentTheme]);

  return (
    <ThemeContext.Provider value={contextValue}>
      {children}
    </ThemeContext.Provider>
  );
};
