# BirdNET-Go Agent Notes

Read the closest instruction file before editing code:
- Root: `CLAUDE.md`
- Go/backend: `internal/CLAUDE.md`
- API work: `internal/api/v2/CLAUDE.md`
- Frontend: `frontend/CLAUDE.md`
- Tests: `TESTING.md`

## Source-of-truth gotchas

- Trust `go.mod` and CI over older prose: the repo targets Go `1.26.1`; CI uses Go `1.26` and Node `24`.
- `go.mod` also declares `toolchain go1.25.3` — this is a minimum toolchain directive, not the actual version in use.
- `README.md` / `task setup-dev` still mention Go `1.25.x`; do not copy that version forward.
- New HTTP endpoints go in `internal/api/v2/` only. Do not extend API v1.

## Real entrypoints

- App binary entrypoint is `main.go`.
- CLI root is `cmd/root.go`.
- The server command is `serve`; `realtime` is only an alias kept for backward compatibility (`cmd/serve/serve.go`).
- Frontend assets are embedded from `frontend/dist` by default (`frontend/embed.go`, build-tagged `!skipfrontend`).

## Build and dev workflow

- Default build: `task`
- Hot reload/full app: `task dev_server` (runs `air realtime`; `.air.toml` configures the Go build, the `realtime` arg is the cobra subcommand)
- Frontend-only build: `task frontend-build`
- Cross-builds are Task targets such as `task linux_amd64`, `task darwin_arm64`, `task windows_amd64`.

Frontend serving has a non-obvious dev mode:
- If `frontend/dist` contains a valid Vite manifest, the Go server serves UI files from disk instead of embedded assets (`internal/api/static.go`).
- Fast backend+frontend loop: `task frontend-watch` in one terminal, `air realtime` in another, then use `http://localhost:8080/ui/`.
- HMR loop: run backend (`air realtime`) plus `task frontend-dev`, then use `http://localhost:5173/ui/`.

## Verification commands

Prefer the repo wrappers when they encode flags/tooling:
- Go lint: `task lint`
  - This injects TensorFlow Lite CGO flags and uses `--build-tags=noembed,skipfrontend`, matching CI intent better than a plain `golangci-lint run`.
- Go tests: `task test`
  - This runs `go test -tags noembed,skipfrontend ./...` to avoid embedded-model/frontend requirements.
- Frontend read-only checks: `task frontend-lint` or `cd frontend && npm run check:all`
- Frontend tests: `task frontend-test`

`task frontend-quality` is not a pure verifier:
- It runs `format`, `lint --fix`, and `ast:fix` before checks/tests/build.
- Use it when you want auto-fixes; avoid it if you only want to inspect failures in a dirty tree.

## Focused test paths

- Single Go package: `go test -tags noembed,skipfrontend ./path/to/pkg/...`
- CI-style quick Go test run adds `-short` and may depend on MQTT: CI sets `MQTT_TEST_BROKER=tcp://localhost:1883` and starts Mosquitto.
- Testcontainer integration coverage uses `-tags=integration` and Docker/Testcontainers for selected packages under `internal/datastore/v2`, `internal/mqtt`, `internal/spectrogram`, and `internal/testutil/containers`.
- Frontend integration tests expect a backend on `localhost:8080`; use `task integration-test-auto` to let Vitest manage the backend, or start it yourself with `task integration-backend` and then run `task integration-test`.

## Frontend constraints worth remembering

- Svelte 5 runes codebase; follow existing patterns, not Svelte 4 patterns.
- No `any` in TypeScript.
- Do not use inline SVGs; use `@lucide/svelte`.
- When adding i18n keys, update every file under `frontend/static/messages/`, not just `en.json`.

## Go/testing constraints worth remembering

- Use `github.com/tphakala/birdnet-go/internal/errors`, not the standard `errors` package (`internal/CLAUDE.md`).
- All tests must use `testify` assertions/requirements; follow `TESTING.md`.
- Many local/CI commands intentionally use `noembed` and `skipfrontend`; avoid removing those tags unless you actually need embedded assets/models.
- Mock generation uses `mockery` with config in `.mockery.yaml`; never manually edit files in `internal/datastore/mocks/`.
- Use `t.Cleanup()` over `defer` for test resource cleanup; use `t.Helper()` in all test helper functions.

## CI quirks

- `frontend-integration-test.yml` is active and runs browser integration tests against a backend built with `-tags skipfrontend`.
- `frontend-e2e-test.yml` exists but is currently disabled with `if: false`; do not assume Playwright E2E is part of required CI.
- golangci-lint CI uses version `v2.10.1`; the `modernize` linter is disabled due to a known panic with Go 1.26 atomic types.
