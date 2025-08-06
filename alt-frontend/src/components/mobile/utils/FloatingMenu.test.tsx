// @vitest-environment jsdom
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import * as matchers from "@testing-library/jest-dom/matchers";
expect.extend(matchers);

// Mock Next.js Link and navigation hooks and ThemeToggle before importing component
vi.mock("next/link", () => ({
  default: ({ href, children, ...props }: any) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/",
}));

vi.mock("../../ThemeToggle", () => ({
  ThemeToggle: () => <div />,
}));

(globalThis as any).scrollTo = vi.fn();

import { FloatingMenu } from "./FloatingMenu";

const renderWithChakra = (ui: React.ReactElement) => {
  return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
};

describe("FloatingMenu", () => {
  it("allows switching between accordion categories", async () => {
    renderWithChakra(<FloatingMenu />);

    fireEvent.click(screen.getByTestId("floating-menu-button"));

    const articlesTab = await screen.findByTestId("tab-articles");
    fireEvent.click(articlesTab);
    expect(await screen.findByText("Search Articles")).toBeInTheDocument();

    const otherTab = screen.getByTestId("tab-other");
    fireEvent.click(otherTab);
    expect(await screen.findByText("Home")).toBeInTheDocument();
  });
});

