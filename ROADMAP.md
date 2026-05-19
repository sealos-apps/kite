# Roadmap

Kite is in active development. This roadmap is a working priority frame for contributors and agents, not a promise of release dates.

## Current Priorities

- Harden authentication and session failure handling so infrastructure or configuration faults do not look like a customer login task.
- Keep multi-cluster operations reliable: cluster import, cluster switching, namespace scope, and RBAC behavior must remain predictable.
- Improve resource workflows around list/detail views, YAML editing, related resources, logs, terminals, and resource history.
- Keep monitoring practical for real clusters by maintaining Prometheus query performance and clear empty/error states.
- Maintain bilingual English/Chinese UI coverage for visible product surfaces while keeping stored status values stable.

## Near-Term Work

- Expand operational runbooks for common deployment, database, authentication, and Kubernetes connectivity failures.
- Continue polishing resource-specific table columns, status displays, and quick actions.
- Improve test coverage around auth/session redirects, RBAC boundaries, and generic CRD handling.
- Keep docs aligned with Helm chart values, environment variables, and deployment modes.

## Guardrails

- Do not treat the product as a landing page. Kite is an operations dashboard first.
- Do not add compatibility layers for removed behavior unless the user or maintainer explicitly asks for compatibility.
- Do not make database writes during investigation unless the user explicitly requests a database modification.
