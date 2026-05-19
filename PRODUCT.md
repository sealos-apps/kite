# Product

## Register

product

## Users

Kite is for platform engineers, Kubernetes operators, SREs, and cluster administrators who need to inspect and operate Kubernetes resources through a browser. Users are usually inside an active operations workflow: checking cluster health, editing YAML, following logs, opening terminals, adjusting RBAC, or validating application state across clusters.

## Product Purpose

Kite provides a lightweight, modern Kubernetes dashboard for multi-cluster resource management, monitoring, and operations. Success means an operator can understand the current cluster state quickly, make controlled changes with confidence, and troubleshoot common Kubernetes problems without dropping to `kubectl` for every task.

## Brand Personality

Quiet, capable, and operational. Kite should feel like a serious control surface: compact enough for repeated daily use, clear enough for stressful debugging, and polished without becoming decorative.

## Anti-references

Avoid marketing-page composition inside the product, oversized hero sections, decorative gradients, glassmorphism, card grids used as filler, and sidebars on standalone error or setup states where they create false application context. Avoid customer-facing copy that implies a user action can fix an operator or infrastructure fault.

## Design Principles

- Optimize for operations density: tables, filters, status indicators, and resource actions should be scannable before they are expressive.
- Surface the real system state: empty states and errors should explain whether the issue is user permissions, cluster connectivity, database state, or authentication configuration.
- Keep shells proportional to workflow scope: full app chrome belongs to authenticated resource work, while setup, initialization, and fault states should remain standalone.
- Prefer familiar Kubernetes vocabulary over branded abstractions when naming resources, roles, and operational checks.
- Make dangerous actions explicit and reversible-looking only when they actually are.

## Accessibility & Inclusion

Design for keyboard navigation, visible focus states, readable contrast in both light and dark themes, and clear text alternatives for icon-only controls. Operational messages should not rely on color alone because status and severity information must remain readable under color-vision constraints.
