import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// React Testing Library leaves the rendered DOM mounted between tests when
// `globals: false`; do it ourselves so every test starts from a fresh DOM.
afterEach(() => {
  cleanup();
});
