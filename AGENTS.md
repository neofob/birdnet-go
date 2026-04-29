# AGENTS

## Fast Ramp-Up
- Primary build/test orchestrator is `Taskfile.yml`; use `task` commands, not `make` (`Makefile` is explicitly deprecated).
- Effective Go version is `1.26.x` (`go.mod` says `go 1.26.1`; some prose docs still mention older versions).
- Main app entrypoint is `main.go`; CLI wiring is in `cmd/root.go` (active API is `internal/api/v2`).

## Commands You Should Actually Run
- Full native build: `task` (auto-detects platform, builds frontend first via `frontend-build`).
- Backend unit tests (fast path used by repo tasks/CI): `task test` (`go test -tags noembed,skipfrontend ./...`).
- Single package test: `go test -tags noembed,skipfrontend ./internal/<pkg>/...`.
- Go lint (matches repo Task behavior): `task lint` (runs golangci-lint with `--build-tags=noembed,skipfrontend`).
- Frontend install/check/build: `task frontend-install`, `task frontend-lint`, `task frontend-build`.
- Frontend test variants: `task frontend-test`, `task integration-test` (backend must be running), `task integration-test-auto` (backend auto-managed), `task e2e-test`.

## Required Order / CI Parity
- Frontend quality gate in CI is effectively: `typecheck -> format:check -> lint -> lint:css`, then `test:ci`, then `analyze:circular`, then `build`.
- Closest local one-shot is `task frontend-quality` (auto-fixes first, then runs checks/tests/build/audit/dependency analysis).
- Go CI runs `go test -tags noembed,skipfrontend -short ./...` plus separate `-tags=integration` suites; keep tags aligned when reproducing failures.

## Repo-Specific Gotchas
- `skipfrontend` build tag swaps embedded UI for a stub FS (`frontend/embed_skipfrontend.go`); great for lint/tests, never for production artifacts.
- Normal builds embed `frontend/dist` (`frontend/embed.go`), so frontend must be built for production-like binaries.
- Pre-commit hook runs both stacks: Go (`gofmt` + `golangci-lint`) and frontend (`lint-staged` + `npm run typecheck` in `frontend/`).
- If you touch frontend i18n keys, update all locale JSON files in `frontend/static/messages/` and run `npm run i18n:validate`.

## Conventions Worth Keeping
- Backend code convention in this repo: prefer `github.com/tphakala/birdnet-go/internal/errors` instead of stdlib `errors` in internal packages.
- Frontend conventions enforced by project docs/tooling: no inline SVGs (use `@lucide/svelte`), avoid `any`, avoid `toISOString()` for local-date UI logic.
