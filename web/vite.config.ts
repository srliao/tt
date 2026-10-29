import { createReadStream, readdirSync, readFileSync, statSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import tailwindcss from '@tailwindcss/vite';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';

// Excalidraw loads its fonts from `window.EXCALIDRAW_ASSET_PATH` (set in
// src/features/drawings/editor-page.tsx), falling back to a public CDN.
// Self-host them under /assets/excalidraw/ — the only static prefix the Go
// SPA handler serves — so the app keeps working offline. Xiaolai (CJK,
// ~12 MB) is skipped to keep the binary small; CJK text falls back to the CDN.
const EXCALIDRAW_URL_PREFIX = '/assets/excalidraw/';
const SKIPPED_FONT_FAMILIES = new Set(['Xiaolai']);

function excalidrawFonts(): Plugin {
  const require = createRequire(import.meta.url);
  // The package doesn't export package.json; its entry is dist/prod/index.js.
  const fontsDir = path.join(path.dirname(require.resolve('@excalidraw/excalidraw')), 'fonts');

  const listFonts = (): string[] =>
    readdirSync(fontsDir)
      .filter((family) => !SKIPPED_FONT_FAMILIES.has(family))
      .flatMap((family) =>
        readdirSync(path.join(fontsDir, family)).map((file) => path.posix.join(family, file)),
      );

  return {
    name: 'tt:excalidraw-fonts',
    configureServer(server) {
      server.middlewares.use(`${EXCALIDRAW_URL_PREFIX}fonts/`, (req, res, next) => {
        const rel = decodeURIComponent((req.url ?? '').split('?')[0]).replace(/^\/+/, '');
        const file = path.join(fontsDir, rel);
        const family = rel.split('/')[0];
        if (!file.startsWith(fontsDir) || SKIPPED_FONT_FAMILIES.has(family)) return next();
        try {
          if (!statSync(file).isFile()) return next();
        } catch {
          return next();
        }
        res.setHeader('Content-Type', 'font/woff2');
        createReadStream(file).pipe(res);
      });
    },
    generateBundle() {
      for (const rel of listFonts()) {
        this.emitFile({
          type: 'asset',
          fileName: `${EXCALIDRAW_URL_PREFIX.slice(1)}fonts/${rel}`,
          source: readFileSync(path.join(fontsDir, rel)),
        });
      }
    },
  };
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    tailwindcss(),
    react(),
    excalidrawFonts(),
  ],
  resolve: {
    tsconfigPaths: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
