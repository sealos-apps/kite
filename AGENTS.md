# Repository Guidelines

## Project Structure & Module Organization
Kite is a Go backend plus a React/TypeScript frontend.

- `main.go`: backend entrypoint.
- `internal/`: internal bootstrapping helpers (for example `internal/load.go`).
- `pkg/`: core backend modules (`auth`, `cluster`, `handlers`, `middleware`, `rbac`, `utils`, etc.).
- `ui/`: Vite + React frontend; main code is under `ui/src/` (`components`, `pages`, `hooks`, `i18n`, `styles`, `types`).
- `docs/`: VitePress documentation site.
- `charts/kite/`: Helm chart.
- `deploy/`: Kubernetes manifests for direct install.
- `scripts/`: release/version helper scripts.

## Build, Test, and Development Commands
Run from repo root unless noted:

- `make deps`: install frontend dependencies (`pnpm`) and download Go modules.
- `make build`: build frontend static assets and backend binary (`./kite`).
- `make dev`: run backend and Vite dev server together.
- `make run`: start the built backend binary.
- `make lint`: run `go vet`, `golangci-lint`, and frontend ESLint.
- `make format`: run `go fmt` and frontend Prettier.
- `make test`: run backend tests (`go test -v ./...`).
- `cd ui && pnpm run type-check`: strict TypeScript checks.
- `make docs-dev` / `make docs-build`: develop or build docs.

## Coding Style & Naming Conventions
- Go: always format with `go fmt`; keep package names lowercase and focused by domain.
- Backend file names typically use snake_case (example: `cluster_manager.go`).
- Frontend uses TypeScript with strict settings and `@/*` path alias.
- Frontend formatting is Prettier-based: 2 spaces, single quotes, no semicolons, trailing commas (`es5`).
- Keep TS/TSX file names kebab-case (example: `node-status-icon.tsx`); export components in PascalCase.

## Testing Guidelines
- Place Go tests beside implementation files using `*_test.go`.
- Current CI enforces build, lint, and backend tests; no fixed coverage gate is defined.
- Add or update tests for any changed backend logic, middleware behavior, or API handlers.

## Commit & Pull Request Guidelines
- Follow Conventional Commit style seen in history: `feat:`, `fix:`, `chore(deps):`, `release vX.Y.Z`.
- Keep commits scoped and descriptive; reference issues when relevant (for example `#383`).
- PRs should include:
  - concise change summary and motivation,
  - verification steps/commands run (`make lint`, `make test`, `make build`),
  - screenshots for UI changes,
  - docs/chart updates when behavior or deployment changes.

## Security & Configuration Tips
- Do not commit kubeconfig files, tokens, or other secrets.
- Review `SECURITY.md` before reporting or handling vulnerabilities.
- Never execute database write operations unless explicitly requested.
