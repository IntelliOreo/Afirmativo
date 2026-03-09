import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach } from "vitest";

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function createStorageMock(): Storage {
  let store = new Map<string, string>();

  return {
    get length() {
      return store.size;
    },
    clear() {
      store = new Map<string, string>();
    },
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
  };
}

Object.defineProperty(window, "localStorage", {
  value: createStorageMock(),
  writable: true,
});

Object.defineProperty(window, "sessionStorage", {
  value: createStorageMock(),
  writable: true,
});

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  cleanup();
});
