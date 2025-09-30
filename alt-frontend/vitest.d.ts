 
/// <reference types="vitest" />
/// <reference types="@vitest/browser/matchers" />
import type { TestingLibraryMatchers } from "@testing-library/jest-dom/matchers";

declare global {
  namespace Vi {
     
    interface JestAssertion<T = any>
      extends TestingLibraryMatchers<T, void> {}
  }
}
