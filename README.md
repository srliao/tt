# tt

Local-only single-user task tracker with userscript automation.

## Build

```sh
just build
./bin/tt
```

## Layout

- `cmd/` — main binary entry points (`cmd/tt`).
- `internal/` — application packages (config, storage, HTTP handlers, etc.).
- `web/` — frontend assets (Vite + pnpm).
- `docs/superpowers/` — design docs and phased implementation plans.
