export type Theme = "vaporwave" | "alt-paper";

export interface ThemeConfig {
  name: Theme;
  label: string;
  description: string;
}

export interface ThemeContextType {
  currentTheme: Theme;
  toggleTheme: () => void;
  setTheme: (theme: Theme) => void;
  themeConfig: ThemeConfig;
}

export const THEME_CONFIGS: Record<Theme, ThemeConfig> = {
  vaporwave: {
    name: "vaporwave",
    label: "Vaporwave",
    description: "Neon retro-future aesthetic",
  },
  "alt-paper": {
    name: "alt-paper",
    label: "Alt Paper",
    description: "Earthy luxury design",
  },
} as const;
