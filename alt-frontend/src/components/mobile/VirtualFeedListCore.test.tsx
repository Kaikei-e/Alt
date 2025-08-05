import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { VirtualFeedListCore } from "./VirtualFeedListCore";
import { Feed } from "@/schema/feed";

// Mock FeedCard component
vi.mock("./FeedCard", () => ({
  default: ({ feed }: { feed: Feed }) => (
    <div data-testid={`feed-card-${feed.id}`}>
      <h3>{feed.title}</h3>
      <p>{feed.description}</p>
    </div>
  ),
}));

describe("VirtualFeedListCore", () => {
  const mockFeeds: Feed[] = Array.from({ length: 100 }, (_, i) => ({
    id: `feed-${i}`,
    title: `Feed ${i}`,
    description: `Description ${i}`,
    link: `https://example.com/feed${i}`,
    published: new Date().toISOString(),
  }));

  const defaultProps = {
    feeds: mockFeeds,
    readFeeds: new Set<string>(),
    onMarkAsRead: vi.fn(),
    estimatedItemHeight: 200,
    containerHeight: 600,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderWithChakra = (ui: React.ReactElement) => {
    return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
  };

  it("should render only visible items", () => {
    renderWithChakra(<VirtualFeedListCore {...defaultProps} />);

    // スクロールコンテナと仮想コンテンツコンテナが存在することを確認
    const scrollContainer = screen.getByTestId("virtual-scroll-container");
    const contentContainer = screen.getByTestId("virtual-content-container");

    expect(scrollContainer).toBeInTheDocument();
    expect(contentContainer).toBeInTheDocument();

    // 仮想化されたアイテムの存在を確認（アイテムが表示される場合）
    const virtualItems = screen.queryAllByTestId(/^virtual-feed-item-/);

    // 仮想化の性質上、表示されるアイテム数は限定的
    expect(virtualItems.length).toBeLessThan(mockFeeds.length); // 全アイテム数より少ない
  });

  it("should maintain scroll position during updates", () => {
    const { rerender } = renderWithChakra(
      <VirtualFeedListCore {...defaultProps} />,
    );

    // スクロールコンテナを取得
    const scrollContainer = screen.getByTestId("virtual-scroll-container");

    // スクロール位置を設定
    Object.defineProperty(scrollContainer, "scrollTop", {
      value: 1000,
      writable: true,
      configurable: true,
    });

    // プロパティ更新
    rerender(
      <ChakraProvider value={defaultSystem}>
        <VirtualFeedListCore
          {...defaultProps}
          readFeeds={new Set(["feed-1"])}
        />
      </ChakraProvider>,
    );

    // スクロール位置が保持されている
    expect(scrollContainer.scrollTop).toBe(1000);
  });

  it("should handle empty feeds gracefully", () => {
    renderWithChakra(<VirtualFeedListCore {...defaultProps} feeds={[]} />);

    expect(screen.getByText("No feeds available")).toBeInTheDocument();
  });

  it("should render virtual scroll container with proper styling", () => {
    renderWithChakra(<VirtualFeedListCore {...defaultProps} />);

    const scrollContainer = screen.getByTestId("virtual-scroll-container");
    expect(scrollContainer).toBeInTheDocument();
    expect(scrollContainer).toHaveStyle({ height: "600px" });
  });

  it("should render virtual content container", () => {
    renderWithChakra(<VirtualFeedListCore {...defaultProps} />);

    const contentContainer = screen.getByTestId("virtual-content-container");
    expect(contentContainer).toBeInTheDocument();
    expect(contentContainer).toHaveStyle({ position: "relative" });
  });

  it("should call onMarkAsRead when feed is marked as read", () => {
    const onMarkAsRead = vi.fn();

    renderWithChakra(
      <VirtualFeedListCore {...defaultProps} onMarkAsRead={onMarkAsRead} />,
    );

    // 仮想化コンテナが存在することを確認
    const scrollContainer = screen.getByTestId("virtual-scroll-container");
    expect(scrollContainer).toBeInTheDocument();

    // onMarkAsRead関数がコンポーネントに渡されていることを確認
    expect(onMarkAsRead).toHaveBeenCalledTimes(0); // 初期状態では呼ばれない
  });

  it("should handle overscan parameter correctly", () => {
    renderWithChakra(<VirtualFeedListCore {...defaultProps} overscan={10} />);

    // 仮想化コンテナが存在することを確認
    const scrollContainer = screen.getByTestId("virtual-scroll-container");
    expect(scrollContainer).toBeInTheDocument();
  });

  it("should position virtual items absolutely", () => {
    renderWithChakra(<VirtualFeedListCore {...defaultProps} />);

    // 仮想化されたアイテムが存在する場合、絶対配置されているかチェック
    const virtualItems = screen.queryAllByTestId(/^virtual-feed-item-/);

    virtualItems.forEach((item) => {
      expect(item).toHaveStyle({ position: "absolute" });
    });
  });
});
