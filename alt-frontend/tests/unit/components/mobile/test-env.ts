import { beforeAll, afterAll, afterEach, vi } from "vitest";

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

const intersectionObserverMock = vi.fn(() => createObserverStub());
const resizeObserverMock = vi.fn(() => createObserverStub());

const originalIntersectionObserver = globalThis.IntersectionObserver;
const originalResizeObserver = (
  globalThis as typeof globalThis & {
    ResizeObserver?: typeof ResizeObserver;
  }
).ResizeObserver;

beforeAll(() => {
  Object.defineProperty(globalThis, "IntersectionObserver", {
    configurable: true,
    writable: true,
    value: intersectionObserverMock,
  });

  Object.defineProperty(globalThis, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: resizeObserverMock,
  });
});

afterEach(() => {
  intersectionObserverMock.mockClear();
  resizeObserverMock.mockClear();
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

export { intersectionObserverMock, resizeObserverMock };
