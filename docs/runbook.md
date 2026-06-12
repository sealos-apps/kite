# Runbook

## Local Development

Install dependencies:

```bash
make deps
```

Run backend and Vite together:

```bash
make dev
```

Default local endpoints:

- Backend: `http://localhost:8080`
- Vite dev server: `http://localhost:5173`
- Health check: `http://localhost:8080/healthz`

## Build And Test

Build frontend static assets and the Go binary:

```bash
make build
```

Run backend tests:

```bash
make test
```

Run lint:

```bash
make lint
```

Run frontend type checks:

```bash
cd ui && pnpm run type-check
```

Build documentation:

```bash
make docs-build
```

## Important Environment Variables

- `PORT`: HTTP listen port, default `8080`.
- `KITE_BASE`: optional subpath base for deployments such as `/kite`.
- `DB_TYPE`: one of `sqlite`, `mysql`, or `postgres`.
- `DB_DSN`: database DSN. Defaults to `dev.db` for SQLite.
- `DB_AUTO_CREATE`: whether Kite should create missing MySQL/PostgreSQL databases before migration. Defaults to `true`.
- `JWT_SECRET`: signing secret for Kite sessions.
- `KITE_ENCRYPT_KEY`: encryption key for sensitive stored values.
- `KUBECONFIG`: source path for first-run cluster import when no clusters exist.
- `ANONYMOUS_USER_ENABLED`: bypass normal auth. Do not enable in production unless the deployment is intentionally trusted.
- `SEALOS_AUTH_ENABLED`: enables the Sealos login API.
- `SEALOS_JWT_SECRET`: JWT secret used for Sealos auth validation.
- `AUTH_COOKIE_SAMESITE` and `AUTH_COOKIE_SECURE`: cookie settings for normal and iframe deployments.
- `VITE_SEALOS_AUTO_LOGIN`: frontend build-time flag controlling Sealos SDK session login attempts.

Sealos auth notes:

- Kite does not block Sealos auth solely because the app is opened as the top-level window. Standalone local development and `sealos-app-dev-bridge` can still attempt Sealos SDK auto-login when `SEALOS_AUTH_ENABLED=true`.
- `/login` can show a non-blocking Sealos SDK availability notice when the SDK session channel is unavailable. Treat it as a diagnostic hint for Sealos Desktop or `sealos-app-dev-bridge`, not as an access gate.
- If Sealos auto-login fails in standalone/local development, verify the bridge or Sealos Desktop session first, then inspect `/api/auth/login/sealos` responses and backend logs.

## Auth Fault Page

`/login` is now an operational fault page. If a customer sees it, treat it as a signal to inspect server-side state instead of asking them to sign in manually.

Common reason codes:

- `unauthenticated`: frontend could not find a valid session.
- `session_refresh_failed`: cookie/session refresh failed.
- `authentication_failed`: an API auth retry failed.
- `insufficient_permissions`: the authenticated user has no matching Kite RBAC access.
- `token_exchange_failed`, `user_info_failed`, `jwt_generation_failed`, `callback_failed`, `callback_error`: OAuth callback or session creation failed.
- `user_disabled`: the Kite user exists but is disabled.

Suggested checks:

1. Inspect backend logs for database, migration, JWT, OAuth, Sealos auth, or session creation errors.
2. Verify `DB_TYPE`, `DB_DSN`, and database account permissions. If `DB_AUTO_CREATE=true`, the account needs database creation permission for MySQL/PostgreSQL.
3. Verify `JWT_SECRET`, `KITE_ENCRYPT_KEY`, `SEALOS_AUTH_ENABLED`, `SEALOS_JWT_SECRET`, and OAuth provider callback settings.
4. Confirm the browser receives and sends the `auth_token` cookie. For iframe/cross-site deployments, check `AUTH_COOKIE_SAMESITE=none`, `AUTH_COOKIE_SECURE=true`, and HTTPS.
5. For `insufficient_permissions`, check Kite RBAC role assignments and OAuth group/user mapping.

## Database Troubleshooting

Startup database failures usually happen before the full UI is usable. Look for panic messages such as:

- `failed to ensure database exists`
- `failed to connect database`
- `failed to migrate database`
- `database connection is nil`

For SQLite hostPath issues, see `docs/faq.md`. For production persistence, prefer MySQL or PostgreSQL.

## Kubernetes Connectivity Troubleshooting

- If managed Kubernetes kubeconfigs use an `exec` plugin such as `aws`, `gcloud`, or `kubelogin`, use a Service Account token kubeconfig instead. See `docs/config/managed-k8s-auth.md`.
- If resource access fails with permission errors, verify Kite RBAC first, then Kubernetes RBAC for the service account or imported kubeconfig.
- If a production image has no shell, use a temporary debug/client pod rather than `kubectl exec` into Kite.

## Production Image Notes

For production deployments, build and publish `linux/amd64` images by default unless ARM is explicitly requested.
