/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import React from "react";
import { ThemeProvider } from "../providers/ThemeProvider";
import { ThemeToggle } from "../components/ThemeToggle";

// Mock ChakraUI components for testing
vi.mock("@chakra-ui/react", () => ({
  Button: ({ children, onClick, onKeyDown, css, ...props }: any) => (
    <button onClick={onClick} onKeyDown={onKeyDown} style={css} {...props}>
      {children}
    </button>
  ),
  Text: ({ children, ...props }: any) => <span {...props}>{children}</span>,
  VStack: ({ children, ...props }: any) => <div {...props}>{children}</div>,
  Box: ({ children, ...props }: any) => <div {...props}>{children}</div>,
  useBreakpointValue: () => "md",
}));

// Mock Lucide icons
vi.mock("lucide-react", () => ({
  Sun: ({ style }: any) => <svg data-testid="sun-icon" style={style} />,
  Moon: ({ style }: any) => <svg data-testid="moon-icon" style={style} />,
}));

// Polyfill window.matchMedia for jsdom environment
if (!window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
}

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
};

Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  writable: true,
});

// Mock document.body.setAttribute to track DOM changes
const setAttributeMock = vi.fn();
Object.defineProperty(document.body, "setAttribute", {
  value: setAttributeMock,
  writable: true,
});

// Mock document.body.getAttribute to simulate theme state
const getAttributeMock = vi.fn();
Object.defineProperty(document.body, "getAttribute", {
  value: getAttributeMock,
  writable: true,
});

// Mock getComputedStyle for CSS variable testing
const originalGetComputedStyle = window.getComputedStyle;
const mockGetComputedStyle = vi.fn();
Object.defineProperty(window, "getComputedStyle", {
  value: mockGetComputedStyle,
  writable: true,
});

describe("Theme Integration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.getItem.mockReturnValue(null);
    getAttributeMock.mockReturnValue("liquid-beige");

    // Setup default CSS variables for liquid-beige theme
    mockGetComputedStyle.mockReturnValue({
      getPropertyValue: vi.fn((prop: string) => {
        const cssVars: Record<string, string> = {
          "--foreground": "#2c2c2c",
          "--background": "#d1c0a8",
          "--accent-primary": "#b85450",
          "--accent-secondary": "#7c9070",
          "--accent-tertiary": "#d4a574",
        };
        return cssVars[prop] || "";
      }),
    });

    cleanup();
  });

  afterEach(() => {
    cleanup();
    // Restore original getComputedStyle
    window.getComputedStyle = originalGetComputedStyle;
  });

  it("デフォルトでliquid-beigeテーマが適用される", async () => {
    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );

    // Wait for component to mount and theme to be resolved
    await waitFor(() => {
      expect(screen.getByTestId("theme-toggle-button")).toBeDefined();
    });

    // Verify default theme is liquid-beige
    const toggleButton = screen.getByTestId("theme-toggle-button");
    expect(toggleButton.getAttribute("aria-checked")).toBe("false"); // liquid-beige = false (not vaporwave)

    // Wait for the component to fully mount and show icon
    await waitFor(() => {
      expect(screen.getByTestId("sun-icon")).toBeDefined();
    });
  });
});
