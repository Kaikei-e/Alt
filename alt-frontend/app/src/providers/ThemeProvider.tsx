"use client";

import React from "react";
import {
  ThemeProvider as NextThemesProvider,
  useTheme as useNextTheme,
} from "next-themes";
import { ThemeContext } from "../contexts/ThemeContext";
import { THEME_CONFIGS, type Theme } from "../types/theme";

// Add constant for localStorage key
const LOCAL_STORAGE_KEY = "alt-theme";

// A client component that bridges the next-themes context to our custom ThemeContext
const ThemeBridge = ({ children }: { children: React.ReactNode }) => {
  const { theme, setTheme, resolvedTheme } = useNextTheme();
  const [mounted, setMounted] = React.useState(false);

  // Use resolvedTheme to ensure we always have a concrete value (light/dark)
  // Map next-themes values to our custom theme names - liquid-beige as default/light theme
  const currentTheme = (
    resolvedTheme === "dark" ? "vaporwave" : "liquid-beige"
  ) as Theme;

  // Local state to reflect current theme synchronously
  const [innerTheme, setInnerTheme] = React.useState<Theme>(currentTheme);

  // Keep in sync when resolvedTheme changes externally
  React.useEffect(() => {
    setInnerTheme(currentTheme);
  }, [currentTheme]);

  // Set mounted state after first render to prevent hydration mismatch
  React.useEffect(() => {
    setMounted(true);
  }, []);

  // Restore theme from localStorage on first mount
  React.useEffect(() => {
    if (typeof window === "undefined") return;
    const stored = window.localStorage.getItem(
      LOCAL_STORAGE_KEY,
    ) as Theme | null;
    if (!stored) return;

    const desired = stored === "vaporwave" ? "dark" : "light"; // value for next-themes
    if (desired !== theme) {
      setTheme(desired);
    }
  }, [setTheme, theme]);

  // Utility to update DOM + storage synchronously
  const updateDomAttributes = (themeName: Theme) => {
    if (typeof document === "undefined") return;
    const root = document.documentElement;
    root.setAttribute("data-style", themeName);
    document.body.setAttribute("data-style", themeName);
    document.body.setAttribute("data-theme", themeName);
    document.body.classList.remove("vaporwave", "liquid-beige");
    document.body.classList.add(themeName);
    try {
      window.localStorage.setItem(LOCAL_STORAGE_KEY, themeName);
    } catch {
      /* ignore */
    }
  };

  // Update body attributes whenever theme changes (reactive)
  React.useEffect(() => {
    if (!mounted) return;
    updateDomAttributes(currentTheme);
  }, [currentTheme, mounted]);

  const contextValue = {
    currentTheme: innerTheme,
    toggleTheme: () => {
      const nextTheme =
        currentTheme === "liquid-beige" ? "vaporwave" : "liquid-beige";
      // Update DOM immediately for snappy UX & tests
      updateDomAttributes(nextTheme);
      setInnerTheme(nextTheme);
      setTheme(nextTheme === "vaporwave" ? "dark" : "light");
    },
    setTheme: (newTheme: Theme) => {
      updateDomAttributes(newTheme);
      setInnerTheme(newTheme);
      setTheme(newTheme === "vaporwave" ? "dark" : "light");
    },
    themeConfig: THEME_CONFIGS[innerTheme],
  };

  return (
    <ThemeContext.Provider value={contextValue}>
      {children}
    </ThemeContext.Provider>
  );
};

export const ThemeProvider = ({ children }: { children: React.ReactNode }) => {
  return (
    <NextThemesProvider
      attribute="data-style"
      defaultTheme="light"
      enableSystem={false}
      // Map our app themes to next-themes' light/dark mode
      themes={["light", "dark"]}
      value={{
        light: "liquid-beige",
        dark: "vaporwave",
      }}
    >
      <ThemeBridge>{children}</ThemeBridge>
    </NextThemesProvider>
  );
};
