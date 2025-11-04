import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWindowSize } from "../../../src/hooks/useWindowSize";

describe("useWindowSize", () => {
  let mockWindowWidth: number;
  let mockWindowHeight: number;

  beforeEach(() => {
    mockWindowWidth = 1024;
    mockWindowHeight = 768;

    // Mock window.innerWidth and window.innerHeight
    Object.defineProperty(window, "innerWidth", {
      writable: true,
      configurable: true,
      value: mockWindowWidth,
    });

    Object.defineProperty(window, "innerHeight", {
      writable: true,
      configurable: true,
      value: mockWindowHeight,
    });

    // Mock window.addEventListener and window.removeEventListener
    vi.spyOn(window, "addEventListener");
    vi.spyOn(window, "removeEventListener");
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("should return initial window size", () => {
    const { result } = renderHook(() => useWindowSize());

    expect(result.current.width).toBe(1024);
    expect(result.current.height).toBe(768);
  });

  it("should register resize event listener", () => {
    renderHook(() => useWindowSize());

    expect(window.addEventListener).toHaveBeenCalledWith("resize", expect.any(Function));
  });

  it("should update window size on resize", () => {
    const { result } = renderHook(() => useWindowSize());

    // Change window size
    mockWindowWidth = 800;
    mockWindowHeight = 600;

    Object.defineProperty(window, "innerWidth", {
      writable: true,
      configurable: true,
      value: mockWindowWidth,
    });

    Object.defineProperty(window, "innerHeight", {
      writable: true,
      configurable: true,
      value: mockWindowHeight,
    });

    // Trigger resize event
    act(() => {
      window.dispatchEvent(new Event("resize"));
    });

    expect(result.current.width).toBe(800);
    expect(result.current.height).toBe(600);
  });

  it("should cleanup event listener on unmount", () => {
    const { unmount } = renderHook(() => useWindowSize());

    unmount();

    expect(window.removeEventListener).toHaveBeenCalledWith("resize", expect.any(Function));
  });

  it("should handle multiple resize events", () => {
    const { result } = renderHook(() => useWindowSize());

    // First resize
    Object.defineProperty(window, "innerWidth", { value: 800 });
    Object.defineProperty(window, "innerHeight", { value: 600 });

    act(() => {
      window.dispatchEvent(new Event("resize"));
    });

    expect(result.current.width).toBe(800);
    expect(result.current.height).toBe(600);

    // Second resize
    Object.defineProperty(window, "innerWidth", { value: 1200 });
    Object.defineProperty(window, "innerHeight", { value: 900 });

    act(() => {
      window.dispatchEvent(new Event("resize"));
    });

    expect(result.current.width).toBe(1200);
    expect(result.current.height).toBe(900);
  });
});
