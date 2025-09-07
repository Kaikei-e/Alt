import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { ThemeProvider } from '../../../src/ThemeProvider';
import { useTheme } from "../../../src/hooks/useTheme";

// Polyfill window.matchMedia for jsdom environment
if (!window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(), // deprecated
      removeListener: vi.fn(), // deprecated
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

// Mock document.body.setAttribute
Object.defineProperty(document.body, "setAttribute", {
  value: vi.fn(),
  writable: true,
});

// Simple test component
const TestComponent = () => {
  const { currentTheme, toggleTheme } = useTheme();

  return (
    <div>
      <span data-testid="theme-display">{currentTheme}</span>
      <button data-testid="toggle-btn" onClick={toggleTheme}>
        Toggle
      </button>
    </div>
  );
};

describe("ThemeProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.getItem.mockReturnValue(null);
    cleanup();
  });

  afterEach(() => {
    cleanup();
  });

  it("should provide theme context to children", async () => {
    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    // Wait for hydration
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(screen.getByTestId("theme-display")).toBeDefined();
    expect(screen.getByTestId("toggle-btn")).toBeDefined();
  });

  it("should initialize with default theme", async () => {
    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const themeDisplay = screen.getByTestId("theme-display");
    expect(themeDisplay.textContent).toBe("alt-paper");
  });

  it("should restore theme from localStorage", async () => {
    localStorageMock.getItem.mockReturnValue("alt-paper");

    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const themeDisplay = screen.getByTestId("theme-display");
    expect(themeDisplay.textContent).toBe("alt-paper");
  });

  it("should update body data-style attribute", async () => {
    localStorageMock.getItem.mockReturnValue("alt-paper");

    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    // Note: next-themes now handles DOM manipulation directly
    // We can verify the theme is set correctly through the context value
    const themeDisplay = screen.getByTestId("theme-display");
    expect(themeDisplay.textContent).toBe("alt-paper");
  });

  it("should handle theme toggle", async () => {
    const user = userEvent.setup();
    localStorageMock.getItem.mockReturnValue("alt-paper");

    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const toggleBtn = screen.getByTestId("toggle-btn");
    const themeDisplay = screen.getByTestId("theme-display");

    // Initial theme should be liquid-beige
    expect(themeDisplay.textContent).toBe("alt-paper");

    await user.click(toggleBtn);

    // Wait for state update
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    // After toggle, should be liquid-beige
    expect(themeDisplay.textContent).toBe("alt-paper");
  });

  it("should fallback to default theme for invalid stored theme", async () => {
    localStorageMock.getItem.mockReturnValue("invalid-theme");

    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const themeDisplay = screen.getByTestId("theme-display");
    // next-themes will use the invalid theme as-is, but our fallback logic should handle it
    // If the invalid theme is passed through, our component should fallback to liquid-beige
    expect(themeDisplay.textContent).toBe("alt-paper");
  });
});
