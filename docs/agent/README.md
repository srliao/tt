# Agent Development Guide — `tt`

Concise, navigation-first docs for AI agents working on this repo. Each file is short and focused; read only the ones relevant to your task.

`tt` is a **local-only single-user task tracker with userscript automation**, shipping as one statically-linked Go binary with an embedded React SPA.

## Where to start

| If your task touches… | Read |
|---|---|
| Anything (always) | [01-architecture.md](./01-architecture.md) — process model, layer map, dependency rules |
| Bugs / "where does X live?" | [02-navigation.md](./02-navigation.md) — file/symbol locator tables |
| DB schema, migrations, sqlc | [03-data-layer.md](./03-data-layer.md) |
| Task / tag / script / run domain logic | [04-backend-services.md](./04-backend-services.md) |
| Userscript execution, `ctx` API, sandbox | [05-runtime.md](./05-runtime.md) |
| Background ticker / job queue | [06-scheduler.md](./06-scheduler.md) |
| HTTP endpoints, validation, error shape | [07-http-api.md](./07-http-api.md) |
| React SPA, routes, components, queries | [08-frontend.md](./08-frontend.md) |
| Build, embed, dev loop, justfile | [09-build-and-dev.md](./09-build-and-dev.md) |
| Writing or running tests | [10-testing.md](./10-testing.md) |
| Common change recipes | [11-common-changes.md](./11-common-changes.md) |

## Canonical references

- **README** (positioning, build commands): repo root
- **Userscript API** (public reference for the `ctx` surface): `docs/userscript-api.md`

When in doubt about "what changed recently?", use `git log`. When in doubt about a design rule, the file under `docs/agent/` that owns the layer is the canonical answer.
