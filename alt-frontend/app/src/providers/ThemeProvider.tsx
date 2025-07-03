"use client";

import React, { useEffect } from "react";
import { useTheme as useNextTheme } from "next-themes";
import { ThemeContext } from "@/contexts/ThemeContext";
import { THEME_CONFIGS, type Theme } from "@/types/theme";

export const ThemeProvider = ({ children }: { children: React.ReactNode }) => {
  const { theme, setTheme } = useNextTheme();

  // Ensure the theme is valid, fallback to liquid-beige
  const isValidTheme = (t: string | undefined): t is Theme =>
    t === "vaporwave" || t === "liquid-beige";
  const currentTheme = isValidTheme(theme) ? theme : "liquid-beige";

  const contextValue = {
    currentTheme,
    toggleTheme: () => {
      const nextTheme =
        currentTheme === "liquid-beige" ? "vaporwave" : "liquid-beige";
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
