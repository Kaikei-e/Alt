import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "./test-env";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { userEvent } from "@testing-library/user-event";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

// Helper function to render with ChakraProvider
const renderWithProvider = (component: React.ReactElement) => {
  return render(<ChakraProvider value={defaultSystem}>{component}</ChakraProvider>);
};

describe("EmptyFeedState", () => {
  afterEach(() => {
    cleanup();
  });

  it("should render empty state message", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check for main heading
    expect(screen.getByRole("heading", { name: /no feeds yet/i })).toBeInTheDocument();

    // Check for descriptive text
    expect(screen.getByText(/start by adding your first rss feed/i)).toBeInTheDocument();
  });

  it("should display icon", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check that the icon container is present
    const iconContainer = screen.getByTestId("empty-state-icon");
    expect(iconContainer).toBeInTheDocument();
  });

  it("should render call-to-action with correct link", () => {
    renderWithProvider(<EmptyFeedState />);

    // Link component renders as <a> tag with button styling
    // Check for CTA link (not button, as Link wraps the Button component)
    const link = screen.getByRole("link", { name: /add your first feed/i });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute("href", "/mobile/feeds/register");
  });

  it("should be accessible", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check for proper ARIA labels
    const container = screen.getByRole("region");
    expect(container).toBeInTheDocument();
  });

  it("should handle link hover states", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderWithProvider(<EmptyFeedState />);

    // Link component renders as <a> tag and should be interactive
    const link = screen.getByRole("link", { name: /add your first feed/i });

    // Link should be interactive
    await user.hover(link);
    expect(link).toBeInTheDocument();
  });
});
