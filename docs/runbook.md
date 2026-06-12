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

Kite uses Helm v4 and requires Go 1.26 or newer for backend builds. Keep local Go, Dockerfile, and CI `GO_VERSION` aligned.

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

Run the full frontend production build:

```bash
cd ui && COREPACK_ENABLE_AUTO_PIN=0 pnpm run build
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

AI and terminal feature state is stored in the admin general settings record rather than plain environment variables:

- `aiAgentEnabled`, `aiProvider`, `aiModel`, `aiApiKey`, `aiBaseUrl`, and `aiMaxTokens` configure the optional AI assistant.
- `kubectlEnabled`, `kubectlImage`, and `nodeTerminalImage` configure optional terminal helper pods.

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

## AI Assistant Operations

- The AI assistant is disabled until an admin enables it in general settings and provides an OpenAI-compatible or Anthropic-compatible provider configuration.
- Read-only tools still use the current authenticated user, cluster, and namespace scope.
- Mutating tools require both Kite RBAC and an explicit continue/confirmation step. Pending sessions are scoped to the same user and cluster.
- If AI chat returns a disabled/provider error, check the general settings record, provider base URL, API key, and model name.

## Helm Operations

- Chart catalog read APIs live under `/api/v1/charts` for authenticated users. Repository create/delete and catalog management APIs remain admin-only under `/api/v1/admin/charts`; stored repository credentials must not be exposed in responses.
- Helm release APIs use the canonical `helmreleases` resource path. The legacy `helmrelease` route remains for compatibility.
- Helm install, upgrade, rollback, uninstall, and auto-upgrade render target manifests before writing. Added resources require `create`, retained resources require `update`, and removed resources require `delete` on the rendered Kubernetes resource.
- Cluster-scoped rendered resources such as CRDs, Namespaces, ClusterRoles, and ClusterRoleBindings require the Kite admin role and are rejected on namespace-scoped Sealos clusters.
- If an upgrade or rollback fails with a rendered resource permission error, inspect the dry-run manifest diff and grant the specific resource verb in Kite RBAC or choose a chart/values set that stays inside the user's namespace scope.

## Kubectl Terminal Operations

- Kubectl terminal is disabled by default and is admin-only.
- Before enabling it, create a ServiceAccount named `kite-kubectl-admin` in the agent namespace with exactly the Kubernetes RBAC the deployment intends to expose.
- Kite creates and cleans up per-session kubectl pods, but it must not auto-create cluster-admin bindings.
- If the terminal fails immediately, check the general settings flag, the configured kubectl image, and whether the `kite-kubectl-admin` ServiceAccount exists in the agent namespace.

## Production Image Notes

For production deployments, build and publish `linux/amd64` images by default unless ARM is explicitly requested.
