# Environment Variables

Kite supports several environment variables by default to change the default values of some configuration items.

- **KITE_USERNAME**: Set the initial administrator username. Can be created through the initialization page
- **KITE_PASSWORD**: Set the initial administrator password. Can be created through the initialization page
- **KUBECONFIG**: Kubernetes configuration file path, default value is `~/.kube/config`. When kite has no configured clusters, it will discover and import clusters from this path by default. Can import clusters through the initialization page
- **ANONYMOUS_USER_ENABLED**: Enable anonymous user access, default value is `false`. When enabled, all access will no longer require authentication and will have the highest permissions by default.

- **JWT_SECRET**: Secret key used for signing and verifying JWT
- **KITE_ENCRYPT_KEY**: Secret key used for encrypting sensitive data, such as user passwords, OAuth clientSecret, kubeconfig, etc.

- **AUTH_COOKIE_SAMESITE**: SameSite policy for auth cookies. Supported values: `lax`, `strict`, `none`. Default is `lax`. For cross-site iframe embedding (for example Sealos), set this to `none`.
- **AUTH_COOKIE_SECURE**: Secure policy for auth cookies. Supported values: `auto`, `true`, `false`. Default is `auto`. If `AUTH_COOKIE_SAMESITE=none`, this must be `true` and your site must use HTTPS.

- **SEALOS_AUTH_ENABLED**: Enable Sealos login API (`/api/auth/login/sealos`), default is `false`.
- **SEALOS_JWT_SECRET**: Secret used to verify Sealos JWT. Kite uses `HS256` for Sealos token verification.
- **SEALOS_DEFAULT_PROMETHEUS_URL**: Default Prometheus URL for Sealos-managed clusters. If set, Kite writes this value for newly synced Sealos clusters and backfills existing Sealos clusters with empty `prometheus_url` during startup.
- **KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES**: Comma-separated namespace allowlist (for example `ns-admin,platform-admin`). For these namespaces, Kite bypasses kubeconfig `current-context.namespace` namespace-scope lock, and Sealos SSO auto-roles are granted `*` namespaces on their managed cluster. Use this only for namespaces whose credentials are truly cluster-admin/global.

- **HOST**: Used for generating OAuth 2.0 authorization callback addresses, default will be obtained from request headers. If you find the result not as expected, you can manually configure this environment variable.

- **NODE_TERMINAL_IMAGE**: Docker image used for generating Node Terminal Agent.

- **ENABLE_ANALYTICS**: Enable data analytics functionality, default value is `false`. When enabled, Kite will collect limited data to help improve the product.

- **PORT**: Port on which Kite runs, default value is `8080`.
- **KITE_DESKTOP_MODE**: Enable desktop mode auto onboarding and auto login. Default is `false`. When `true`, Kite auto-creates a default local admin user (if no users exist) and auto-signs in desktop users.
- **KITE_DESKTOP_DEFAULT_USERNAME**: Default username for desktop auto-created admin user. Effective when `KITE_DESKTOP_MODE=true`. Default is `admin`.
- **KITE_DESKTOP_DEFAULT_NAME**: Default display name for desktop auto-created admin user. Effective when `KITE_DESKTOP_MODE=true`. Default is `Admin`.

Optional frontend environment variables (build-time):

- **VITE_SEALOS_AUTO_LOGIN**: `true` or `false`. Controls whether frontend should auto-attempt Sealos SDK session login. Default is `true`.
