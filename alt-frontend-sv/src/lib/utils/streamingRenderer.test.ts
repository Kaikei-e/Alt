
import { describe, expect, test, mock } from "bun:test";
import { simulateTypewriterEffect } from "./streamingRenderer";

describe("simulateTypewriterEffect", () => {
  test("should emit characters sequentially with delay", async () => {
    const chars: string[] = [];
    const delay = 10;
    const tick = mock(async () => { });

    const typewriter = simulateTypewriterEffect(
      (char) => chars.push(char),
      { delay, tick }
    );

    const start = Date.now();
    typewriter.add("Hello");
    await typewriter.getPromise();

    const duration = Date.now() - start;

    expect(chars.join("")).toBe("Hello");
    // 5 chars, 4 delays of 10ms = 40ms minimum.
    // Allow some buffer.
    expect(duration).toBeGreaterThanOrEqual(40);
    expect(tick).toHaveBeenCalled();
  });

  test("should handle multiple add calls sequentially", async () => {
    const chars: string[] = [];
    const typewriter = simulateTypewriterEffect(
      (char) => chars.push(char),
      { delay: 5 }
    );

    // Fire both immediately (non-blocking)
    typewriter.add("Hi");
    typewriter.add("There");

    await typewriter.getPromise();

    expect(chars.join("")).toBe("HiThere");
  });

  test("should stop when cancelled", async () => {
    const chars: string[] = [];
    const typewriter = simulateTypewriterEffect(
      (char) => chars.push(char),
      { delay: 20 }
    );

    typewriter.add("LongText");

    // Cancel after small delay
    setTimeout(() => {
      typewriter.cancel();
    }, 30);

    await typewriter.getPromise();

    // Should have emitted some chars but not all
    expect(chars.length).toBeLessThan("LongText".length);
    expect(chars.length).toBeGreaterThan(0);
  });
});
