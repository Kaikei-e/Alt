// vitest.setup.ts
import "@testing-library/jest-dom";

// React をグローバルに設定
import React from "react";
import { vi } from "vitest";

global.React = React;

// webidl-conversionsとwhatwg-urlはモックしない
// ベストプラクティス: 外部依存（API、データベース、ネットワーク）のみモック
// これらのライブラリは実際の実装を使用し、jsdom環境で正常に動作するはず

// より包括的なWeb APIポリフィル
if (typeof global.HTMLElement === "undefined") {
  global.HTMLElement = class HTMLElement {} as typeof HTMLElement;
}

if (typeof global.Element === "undefined") {
  global.Element = class Element {} as typeof Element;
}

if (typeof global.Node === "undefined") {
  global.Node = class Node {} as typeof Node;
}

if (typeof global.EventTarget === "undefined") {
  global.EventTarget = class EventTarget {} as typeof EventTarget;
}

// webidl-conversions が期待する Web API のポリフィル
if (typeof global.DOMException === "undefined") {
  const DOMExceptionClass = class DOMException extends Error {
    constructor(message: string, name?: string) {
      super(message);
      this.name = name || "DOMException";
    }
  };
  Object.assign(DOMExceptionClass, {
    INDEX_SIZE_ERR: 1,
    DOMSTRING_SIZE_ERR: 2,
    HIERARCHY_REQUEST_ERR: 3,
    WRONG_DOCUMENT_ERR: 4,
    INVALID_CHARACTER_ERR: 5,
    NO_DATA_ALLOWED_ERR: 6,
    NO_MODIFICATION_ALLOWED_ERR: 7,
    NOT_FOUND_ERR: 8,
    NOT_SUPPORTED_ERR: 9,
    INUSE_ATTRIBUTE_ERR: 10,
    INVALID_STATE_ERR: 11,
    SYNTAX_ERR: 12,
    INVALID_MODIFICATION_ERR: 13,
    NAMESPACE_ERR: 14,
    INVALID_ACCESS_ERR: 15,
    VALIDATION_ERR: 16,
    TYPE_MISMATCH_ERR: 17,
    SECURITY_ERR: 18,
    NETWORK_ERR: 19,
    ABORT_ERR: 20,
    URL_MISMATCH_ERR: 21,
    QUOTA_EXCEEDED_ERR: 22,
    TIMEOUT_ERR: 23,
    INVALID_NODE_TYPE_ERR: 24,
    DATA_CLONE_ERR: 25,
  });
  global.DOMException = DOMExceptionClass as unknown as typeof DOMException;
}

if (typeof global.AbortController === "undefined") {
  global.AbortController = class AbortController {
    signal = { aborted: false };
    abort() {
      this.signal.aborted = true;
    }
  } as typeof AbortController;
}

if (typeof global.AbortSignal === "undefined") {
  global.AbortSignal = class AbortSignal {
    aborted = false;
  } as typeof AbortSignal;
}

// Node.js環境で必要なWeb APIのポリフィル
if (typeof global.TextEncoder === "undefined") {
  const { TextEncoder, TextDecoder } = require("util");
  global.TextEncoder = TextEncoder;
  global.TextDecoder = TextDecoder;
}

// URL と URLSearchParams のポリフィル
if (typeof global.URL === "undefined") {
  const { URL, URLSearchParams } = require("url");
  global.URL = URL;
  global.URLSearchParams = URLSearchParams;
}

// fetch のポリフィル
if (typeof global.fetch === "undefined") {
  global.fetch = require("node-fetch");
}

// document と navigator のモック（webidl-conversions用）
if (typeof global.document === "undefined") {
  global.document = {
    createElement: vi.fn(),
    getElementById: vi.fn(),
    querySelector: vi.fn(),
    querySelectorAll: vi.fn(),
  } as unknown as Document;
}

if (typeof global.navigator === "undefined") {
  global.navigator = {
    userAgent: "test",
    platform: "test",
  } as Navigator;
}

// window.matchMedia のモック（jsdom環境でのみ実行）
if (typeof window !== "undefined") {
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

// ResizeObserver のモック（コンストラクタクラス形式）
class ResizeObserverMock {
  observe: ReturnType<typeof vi.fn>;
  unobserve: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;

  constructor(callback?: ResizeObserverCallback) {
    this.observe = vi.fn();
    this.unobserve = vi.fn();
    this.disconnect = vi.fn();
  }
}

global.ResizeObserver = ResizeObserverMock as unknown as typeof ResizeObserver;

// IntersectionObserver のモック（コンストラクタクラス形式）
class IntersectionObserverMock {
  observe: ReturnType<typeof vi.fn>;
  unobserve: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;

  constructor(
    callback?: IntersectionObserverCallback,
    options?: IntersectionObserverInit,
  ) {
    this.observe = vi.fn();
    this.unobserve = vi.fn();
    this.disconnect = vi.fn();
  }
}

global.IntersectionObserver =
  IntersectionObserverMock as unknown as typeof IntersectionObserver;
