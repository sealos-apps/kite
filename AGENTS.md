# Repository Guidelines

## Project Structure & Module Organization
Kite is a Go backend plus a React/TypeScript frontend.

- `main.go`: backend entrypoint.
- `internal/`: internal bootstrapping helpers (for example `internal/load.go`).
- `pkg/`: core backend modules (`ai`, `auth`, `cluster`, `helm`, `helmutil`, `helmguard`, `handlers`, `middleware`, `rbac`, `scheduler`, `terminal`, `utils`, etc.).
- `ui/`: Vite + React frontend; main code is under `ui/src/` (`components`, `pages`, `hooks`, `i18n`, `styles`, `types`).
- `docs/`: VitePress documentation site.
- `deploy/charts/kite/`: Helm chart used by Sealos packaging and release scripts.
- `deploy/`: Kubernetes manifests for direct install.
- `scripts/`: release/version helper scripts.

## Build, Test, and Development Commands
Run from repo root unless noted:

- `make deps`: install frontend dependencies (`pnpm`) and download Go modules.
- `make build`: build frontend static assets and backend binary (`./kite`).
- `make dev`: run backend and Vite dev server together.
  This target starts the backend with `DISABLE_CACHE=true` so local development
  does not spin up controller-runtime informer caches for every saved cluster.
  If a manual backend run is needed during development, set `DISABLE_CACHE=true`
  unless cache behavior is the thing being tested.
- `make run`: start the built backend binary.
- `make lint`: run `go vet`, `golangci-lint`, and frontend ESLint.
- `make format`: run `go fmt` and frontend Prettier.
- `make test`: run backend tests (`go test -v ./...`).
- `cd ui && pnpm run type-check`: strict TypeScript checks.
- `cd ui && pnpm run build`: strict TypeScript checks plus Vite/Tailwind production build.
- `make docs-dev` / `make docs-build`: develop or build docs.

## Coding Style & Naming Conventions
- Go: always format with `go fmt`; keep package names lowercase and focused by domain.
- Backend file names typically use snake_case (example: `cluster_manager.go`).
- Frontend uses TypeScript with strict settings and `@/*` path alias.
- Frontend formatting is Prettier-based: 2 spaces, single quotes, no semicolons, trailing commas (`es5`).
- Keep TS/TSX file names kebab-case (example: `node-status-icon.tsx`); export components in PascalCase.
- Frontend i18n must preserve existing flat `common.*` keys. Do not convert string keys such as `common.actions` into nested objects; wire new UI to existing flat keys or add non-conflicting feature/nested keys instead.

## Testing Guidelines
- Place Go tests beside implementation files using `*_test.go`.
- Current CI enforces build, lint, and backend tests; no fixed coverage gate is defined.
- Add or update tests for any changed backend logic, middleware behavior, or API handlers.
- When syncing upstream UI, run a literal translation-key scan for the touched scopes in addition to `cd ui && pnpm run type-check` and `cd ui && pnpm run build`, so raw keys such as `common.fields.description` do not leak into the interface.

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
- Helm v4 requires Go 1.26. Keep `go.mod`, Dockerfile, and GitHub Actions `GO_VERSION` in sync.
- AI resource mutation tools and Helm release operations are write-capable; keep their user-confirmation, rendered-manifest guard, namespace-scope checks, and RBAC gates intact.
- AI Agent UI is enabled by default for new installs, but chat runtime still requires a configured API key. The first `/settings` tab is AI Agent-only configuration; keep kubectl, node terminal, analytics, and login-prompt controls out of that visible panel unless the product decision changes explicitly.
- Helm chart catalog read routes are available to authenticated users under `/api/v1/charts`; repository create/delete and other catalog management routes stay admin-only under `/api/v1/admin/charts`. Do not expose stored repository credentials in responses.
- Kubectl terminal is disabled by default, admin-only, and requires a pre-created `kite-kubectl-admin` ServiceAccount in the agent namespace. Kite must not auto-create cluster-admin RBAC.
- Preserve Sealos compatibility when syncing upstream: `/api/auth/login/sealos`, Sealos SDK auto-login, standalone/local dev bridge auto-login, namespace-scoped cluster behavior, `_all` routing, and default Sealos Prometheus backfill are all intentional fork behavior.

## Auth UI Product Decisions
- `/login` is an operational fault page, not an interactive sign-in surface. Do not reintroduce username/password forms, OAuth provider buttons, or a dashboard sidebar there unless the product decision changes explicitly.
- Auth/session failures should redirect to `/login?reason=<code>` and explain that the likely cause is server-side configuration, database, or authentication-service trouble. Keep the page standalone, use the real Kite logo asset, and keep the remediation copy operator-focused.
- Sealos auth must not block solely because Kite is running as the top-level window. Standalone local development, including `sealos-app-dev-bridge`, should be allowed to attempt Sealos SDK auto-login first.
- `/login` may show a small Sealos SDK availability notice when the SDK session channel is unavailable, but that notice is diagnostic-only and must not replace or block the auto-login attempt.
- The interactive authentication APIs still exist (`/api/auth/login/password`, `/api/auth/login`, `/api/auth/callback`, `/api/auth/login/sealos`) for backend/session flows and admin-managed auth configuration.
