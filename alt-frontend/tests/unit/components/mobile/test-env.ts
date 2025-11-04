import { afterAll, afterEach, beforeAll, vi } from "vitest";

type ObserverMock = {
  observe: ReturnType<typeof vi.fn>;
  unobserve: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;
};

const createObserverStub = (): ObserverMock => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
});

// コンストラクタとして機能するクラスを実装
class IntersectionObserverMock {
  observe: ReturnType<typeof vi.fn>;
  unobserve: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;

  constructor(
    callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit
  ) {
    const stub = createObserverStub();
    this.observe = stub.observe;
    this.unobserve = stub.unobserve;
    this.disconnect = stub.disconnect;
  }
}

class ResizeObserverMock {
  observe: ReturnType<typeof vi.fn>;
  unobserve: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;

  constructor(callback: ResizeObserverCallback) {
    const stub = createObserverStub();
    this.observe = stub.observe;
    this.unobserve = stub.unobserve;
    this.disconnect = stub.disconnect;
  }
}

const originalIntersectionObserver = globalThis.IntersectionObserver;
const originalResizeObserver = (
  globalThis as typeof globalThis & {
    ResizeObserver?: typeof ResizeObserver;
  }
).ResizeObserver;

beforeAll(() => {
  // コンストラクタとして正しく機能するモックを確実に設定
  // vitest.setup.tsのモックを上書きするが、同じコンストラクタ形式なので問題ない
  Object.defineProperty(globalThis, "IntersectionObserver", {
    configurable: true,
    writable: true,
    value: IntersectionObserverMock as unknown as typeof IntersectionObserver,
  });

  Object.defineProperty(globalThis, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: ResizeObserverMock as unknown as typeof ResizeObserver,
  });
});

afterEach(() => {
  // モックのクリアは各インスタンスに対して行う必要があるため、ここでは何もしない
  // 各テストで必要に応じてモックをクリアする
});

afterAll(() => {
  Object.defineProperty(globalThis, "IntersectionObserver", {
    configurable: true,
    writable: true,
    value: originalIntersectionObserver,
  });

  Object.defineProperty(globalThis, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: originalResizeObserver,
  });
});

// 後方互換性のため、モック関数としてもエクスポート
const intersectionObserverMock = vi.fn((callback: IntersectionObserverCallback, options?: IntersectionObserverInit) => {
  return new IntersectionObserverMock(callback, options);
});

const resizeObserverMock = vi.fn((callback: ResizeObserverCallback) => {
  return new ResizeObserverMock(callback);
});

export { intersectionObserverMock, resizeObserverMock };
