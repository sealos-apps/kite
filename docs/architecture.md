# Architecture

Kite is a single Go service that serves a React application and exposes REST/WebSocket APIs for Kubernetes operations.

## Runtime Shape

- `main.go` starts a Gin HTTP server on `PORT` (default `8080`), initializes configuration, connects to the database, runs migrations, initializes RBAC and templates, then registers API and static routes.
- The frontend is a Vite React app under `ui/`. Production builds are embedded into the Go binary through `//go:embed static`.
- The backend uses GORM with SQLite, MySQL, or PostgreSQL. `DB_TYPE`, `DB_DSN`, and `DB_AUTO_CREATE` control connection behavior.
- Kubernetes access is managed through `pkg/cluster`, `pkg/kube`, and middleware that resolves the current cluster from authenticated requests.

## Request Flow

1. Browser requests load the embedded React app from `static/index.html`.
2. React Router defines standalone `/setup` and `/login` routes, then protected app routes under `/`.
3. `InitCheckRoute` calls `/api/v1/init_check`; uninitialized installs go to `/setup`.
4. `ProtectedRoute` checks frontend auth state; missing sessions redirect to `/login?reason=unauthenticated`.
5. Protected backend APIs run through `RequireAuth`, cluster middleware, and RBAC middleware before resource handlers.

## Auth And Session Model

- Auth routes live under `/api/auth`.
- Backend interactive auth APIs still include password login, OAuth start/callback, Sealos login, logout, refresh, and current-user lookup.
- The frontend `/login` route is not an interactive login surface. It is an operational fault page used when no usable access session can be established or an auth callback/session refresh fails.
- Session state uses an `auth_token` cookie. `RequireAuth` accepts bearer tokens, API key tokens, or the cookie depending on request type.
- Anonymous mode can bypass normal user auth when `ANONYMOUS_USER_ENABLED=true`, but this is not a production-safe default.

## Data Model

Core persistent models are initialized in `pkg/model/model.go`:

- `User`
- `Cluster`
- `OAuthProvider`
- `Role`
- `RoleAssignment`
- `ResourceHistory`
- `ResourceTemplate`

Kite runs `AutoMigrate` on startup. When `DB_AUTO_CREATE=true`, the service attempts to create a missing MySQL/PostgreSQL target database before migrations, so the configured database user must have the required permission.

## Resource Handling

- Generic Kubernetes resource list/detail/apply routes are registered from `pkg/handlers/resources`.
- Custom Resources are handled through `pkg/handlers/resources/cr_handler.go` and frontend routes under `/crds/...`.
- Search, logs, web terminal, node terminal, image tags, templates, Prometheus metrics, and proxy routes are registered under `/api/v1`.

## Sealos Compatibility Layer

- `/api/auth/login/sealos` remains available when `SEALOS_AUTH_ENABLED=true`.
- Sealos SDK auto-login is handled in the frontend auth context and stores the selected cluster in storage plus the `x-cluster-name` cookie/header.
- The frontend does not reject Sealos auth just because it is running as the top-level window. This keeps standalone local development and tools such as `sealos-app-dev-bridge` on the same SDK auto-login path as iframe deployments.
- `/login` may show a Sealos SDK availability notice when the SDK session channel is unavailable, but the notice is diagnostic-only and does not block the auto-login attempt.

## Static Assets And Base Path

`KITE_BASE` configures a subpath deployment. The Go server redirects `/` to the base path when needed, injects the base into the built HTML, serves `/assets/*` with static caching, and returns the SPA entrypoint for non-API unknown routes.

## Operational Notes

- Database connection and migration failures happen during startup and can prevent the backend from serving normally.
- Session/auth failures in the frontend should send users to `/login?reason=<code>` and present operator remediation rather than a misleading login form.
- Distroless production images may not contain a shell; use a temporary client/debug pod for in-cluster database or network troubleshooting.
