import { describe, it, expect } from "vitest";
import { DesktopFeedsLayoutProps } from "@/types/desktop-feeds";

describe("DesktopFeedsLayout", () => {
  it("should have correct interface definition", () => {
    // 型定義の基本テスト
    const mockProps: DesktopFeedsLayoutProps = {
      children: "test children",
      sidebar: "test sidebar",
      header: "test header",
    };

    expect(mockProps.children).toBe("test children");
    expect(mockProps.sidebar).toBe("test sidebar");
    expect(mockProps.header).toBe("test header");
  });

  it("should accept React nodes as props", () => {
    // React Node型の確認
    const mockProps: DesktopFeedsLayoutProps = {
      children: null,
      sidebar: undefined,
      header: "header text",
    };

    expect(mockProps.children).toBeNull();
    expect(mockProps.sidebar).toBeUndefined();
    expect(mockProps.header).toBe("header text");
  });
});
