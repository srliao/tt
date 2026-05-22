import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// React Testing Library leaves the rendered DOM mounted between tests when
// `globals: false`; do it ourselves so every test starts from a fresh DOM.
afterEach(() => {
  cleanup();
});

// jsdom is missing a few DOM APIs that Radix UI primitives expect to be
// present (ResizeObserver and Element.scrollIntoView are used internally by
// the Select primitive). Stub them so tests that mount portalled selects
// don't crash.
if (typeof window !== 'undefined') {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  if (!('ResizeObserver' in window)) {
    // biome-ignore lint/suspicious/noExplicitAny: minimal jsdom polyfill
    (window as any).ResizeObserver = ResizeObserverStub;
  }
  if (typeof Element !== 'undefined' && !('scrollIntoView' in Element.prototype)) {
    // biome-ignore lint/suspicious/noExplicitAny: minimal jsdom polyfill
    (Element.prototype as any).scrollIntoView = function scrollIntoView() {};
  }
  if (typeof Element !== 'undefined' && !('hasPointerCapture' in Element.prototype)) {
    // biome-ignore lint/suspicious/noExplicitAny: minimal jsdom polyfill
    (Element.prototype as any).hasPointerCapture = function hasPointerCapture() {
      return false;
    };
  }
  if (typeof Element !== 'undefined' && !('releasePointerCapture' in Element.prototype)) {
    // biome-ignore lint/suspicious/noExplicitAny: minimal jsdom polyfill
    (Element.prototype as any).releasePointerCapture = function releasePointerCapture() {};
  }
  // ThemeProvider asks the system colour scheme via matchMedia; jsdom lacks
  // it so default to light + a no-op listener.
  // jsdom doesn't implement window.confirm(); default to "OK" so the
  // unsaved-changes guard doesn't crash during test navigation.
  if (typeof window.confirm !== 'function') {
    // biome-ignore lint/suspicious/noExplicitAny: minimal jsdom polyfill
    (window as any).confirm = () => true;
  }
  if (typeof window.matchMedia !== 'function') {
    // biome-ignore lint/suspicious/noExplicitAny: minimal jsdom polyfill
    (window as any).matchMedia = (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    });
  }
}
