import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import React from "react";
import { useTheme } from "../../../src/hooks/useTheme";
import type { Theme } from "../../../src/types/theme";
import { ThemeProvider } from "../../../src/providers/ThemeProvider";

// Mock localStorage
const mockLocalStorage = (() => {
  let store: Record<string, string> = {};

  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    clear: vi.fn(() => {
      store = {};
    }),
  };
})();

Object.defineProperty(window, "localStorage", {
  value: mockLocalStorage,
});

// Mock the ThemeContext itself
vi.mock("../../../src/contexts/ThemeContext", () => ({
  ThemeContext: React.createContext({
    currentTheme: "vaporwave" as Theme,
    toggleTheme: vi.fn(),
    setTheme: vi.fn(),
    themeConfig: {
      name: "vaporwave" as Theme,
      label: "Vaporwave",
      description: "Neon retro-future aesthetic",
    },
  }),
}));

describe("useTheme", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockLocalStorage.clear();
  });

  it("should return theme context when used within provider", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(ThemeProvider, null, children);

    const { result } = renderHook(() => useTheme(), { wrapper });

    expect(result.current).toBeDefined();
    expect(result.current.currentTheme).toBeDefined();
    expect(result.current.themeConfig).toBeDefined();
  });

  it("should not throw error when used with provider", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(ThemeProvider, null, children);

    expect(() => {
      renderHook(() => useTheme(), { wrapper });
    }).not.toThrow();
  });

  it("should have correct theme configuration", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(ThemeProvider, null, children);

    const { result } = renderHook(() => useTheme(), { wrapper });

    expect(result.current.themeConfig).toHaveProperty("name");
    expect(result.current.themeConfig).toHaveProperty("label");
    expect(result.current.themeConfig).toHaveProperty("description");
  });

  it("should provide toggle and set theme functions", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(ThemeProvider, null, children);

    const { result } = renderHook(() => useTheme(), { wrapper });

    expect(typeof result.current.toggleTheme).toBe("function");
    expect(typeof result.current.setTheme).toBe("function");
  });
});
