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
- `GeneralSetting`
- `PendingSession`
- `HelmRepository`
- `ScheduledTask`

Kite runs `AutoMigrate` on startup. When `DB_AUTO_CREATE=true`, the service attempts to create a missing MySQL/PostgreSQL target database before migrations, so the configured database user must have the required permission.

## Resource Handling

- Generic Kubernetes resource list/detail/apply routes are registered from `pkg/handlers/resources`.
- Custom Resources are handled through `pkg/handlers/resources/cr_handler.go` and frontend routes under `/crds/...`.
- Search, logs, web terminal, node terminal, optional kubectl terminal, image tags, templates, Prometheus metrics, and proxy routes are registered under `/api/v1`.
- `pkg/ai` exposes `/api/v1/ai/chat` plus continuation endpoints. Tools read resources, logs, cluster overview, and Prometheus data through the current user and cluster context. Mutating tools create a pending session and execute only after the user continues the confirmed operation.
- `pkg/helm` exposes authenticated read-only chart catalog APIs under `/api/v1/charts` so namespaced Sealos users can browse and install charts they are authorized to render. Repository create/delete remains admin-only under `/api/v1/admin/charts`, and stored repository credentials must never be exposed in responses.
- `pkg/handlers/resources/helmrelease_handler.go` registers `helmreleases` as the canonical resource route, with a legacy `helmrelease` alias for compatibility. Helm install, upgrade, rollback, uninstall, and auto-upgrade pass through rendered-manifest authorization in `pkg/helmguard` before Helm writes resources.
- `pkg/scheduler` runs background scheduled tasks such as Helm release auto-upgrade. Tasks reload their creator and due/enabled state before execution.

## Static Assets And Base Path

`KITE_BASE` configures a subpath deployment. The Go server redirects `/` to the base path when needed, injects the base into the built HTML, serves `/assets/*` with static caching, and returns the SPA entrypoint for non-API unknown routes.

## Operational Notes

- Database connection and migration failures happen during startup and can prevent the backend from serving normally.
- Session/auth failures in the frontend should send users to `/login?reason=<code>` and present operator remediation rather than a misleading login form.
- Distroless production images may not contain a shell; use a temporary client/debug pod for in-cluster database or network troubleshooting.
- Helm v4 is part of the backend dependency graph; build environments must use Go 1.26 or newer.
- Kubectl terminal creates per-session pods only after an admin enables the feature and pre-provisions the `kite-kubectl-admin` ServiceAccount/RBAC.
