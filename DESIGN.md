---
name: Kite
description: A restrained Kubernetes operations dashboard for dense, repeatable cluster work.
colors:
  background: "#ffffff"
  foreground: "#09090b"
  card: "#ffffff"
  muted: "#f4f4f5"
  muted-foreground: "#71717b"
  border: "#e4e4e7"
  primary: "#007cdd"
  primary-foreground: "#f2fafe"
  destructive: "#e7000b"
typography:
  body:
    fontFamily: "var(--app-font-sans)"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.5
  title:
    fontFamily: "var(--app-font-sans)"
    fontSize: "18px"
    fontWeight: 600
    lineHeight: 1.25
  label:
    fontFamily: "var(--app-font-sans)"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1.25
  mono:
    fontFamily: "var(--font-mono)"
rounded:
  sm: "4px"
  md: "6px"
  lg: "8px"
  xl: "12px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    rounded: "{rounded.md}"
    height: "36px"
    padding: "8px 16px"
  button-outline:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    height: "36px"
    padding: "8px 16px"
  card:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.xl}"
    padding: "24px"
---

# Design System: Kite

## 1. Overview

**Creative North Star: "Operations Console"**

Kite is a product UI, not a marketing surface. The interface should feel calm, utilitarian, and ready for repeated operational use. Density is allowed when it improves scanning, but every dense area needs clear hierarchy, stable spacing, and predictable action placement.

The visual system is based on Tailwind and shadcn-style primitives, using RGB CSS variables exposed through `ui/tailwind.config.ts`. The default light theme is the baseline; dark and color themes should preserve the same component structure and spacing rhythm.

**Key Characteristics:**

- Compact app chrome with a sidebar only inside authenticated resource workflows.
- Neutral surfaces, one blue operational accent, and semantic status colors.
- Tables, selectors, dialogs, YAML editors, logs, terminals, and charts are first-class product surfaces.
- Standalone setup and fault states should stay lightweight and should not mimic the full dashboard shell.

## 2. Colors

The palette is restrained: neutral zinc-like surfaces with a clear blue primary accent used for active controls, focus rings, and selected navigation.

### Primary

- **Operational Blue** (`#007cdd`): primary buttons, active states, and focus affordances. Use it sparingly so important actions stay visible.

### Neutral

- **Canvas White** (`#ffffff`): default page and card backgrounds in the light theme.
- **Console Ink** (`#09090b`): primary body text and high-emphasis labels.
- **Soft Control Grey** (`#f4f4f5`): muted fills, hover states, secondary controls, and inactive regions.
- **Subtle Divider** (`#e4e4e7`): borders, table row separators, and panel outlines.
- **Secondary Text** (`#71717b`): supporting copy, hints, and low-emphasis metadata.

### Semantic

- **Destructive Red** (`#e7000b`): destructive actions and hard error states.
- Use amber for warning/fault emphasis, especially when the system is unavailable but the user is not being blamed.

### Named Rules

**The One Accent Rule.** Blue is the primary action color. Do not introduce competing purple, teal, or gradient accents on operational screens.

## 3. Typography

**Display Font:** system UI through `var(--app-font-sans)`
**Body Font:** system UI through `var(--app-font-sans)`
**Label/Mono Font:** `Maple Mono` through `var(--font-mono)`, with `JetBrains Mono` available as a font asset.

**Character:** Type should read like an operations tool: tight, direct, and clear. Large display type is rare and should be reserved for standalone states, not panels inside the dashboard.

### Hierarchy

- **Title** (600, 18px to 24px, tight line-height): page titles, dialog titles, and standalone state headings.
- **Body** (400, 14px): normal UI copy, table cells, forms, and explanatory text.
- **Label** (500, 12px to 14px): field labels, metadata labels, badges, and compact navigation text.
- **Mono** (`var(--font-mono)`): YAML, logs, commands, resource identifiers, and code-like values.

### Named Rules

**The Panel Scale Rule.** Do not use hero-scale headings inside cards, dialogs, sidebars, or settings panels.

## 4. Elevation

Kite uses borders and tonal layering more than shadows. Shadows are light structural hints (`shadow-xs`, `shadow-sm`) on buttons, cards, dropdowns, and popovers, not decoration. Most depth should come from border contrast, background changes, and fixed layout regions.

### Shadow Vocabulary

- **Control Lift** (`0 1px 2px 0 rgba(0, 0, 0, 0.05)`): small buttons and outlined controls.
- **Panel Lift** (`shadow-sm`): cards, dropdowns, and popovers that need separation from the page.

### Named Rules

**The Flat By Default Rule.** Surfaces are flat at rest; avoid heavy shadows and floating-card layouts for page sections.

## 5. Components

### Buttons

- **Shape:** medium radius (`rounded-md`, roughly 6px).
- **Primary:** blue fill, primary foreground text, 36px default height, icon gap of 8px.
- **Hover / Focus:** subtle background darkening plus a visible focus ring using the `ring` token.
- **Outline / Ghost:** neutral backgrounds with hover fills; use these for secondary commands and toolbar actions.

### Cards / Containers

- **Corner Style:** 12px for generic cards, 8px or less for dense operational panels when the page calls for tighter geometry.
- **Background:** `card` on main surfaces, `muted/20` or `muted/50` for secondary regions.
- **Shadow Strategy:** light panel lift only when the container floats above a page or menu.
- **Border:** use borders consistently for dashboard panels, tables, and fault pages.
- **Internal Padding:** 16px to 24px, reduced only for dense table or toolbar areas.

### Inputs / Fields

- **Style:** neutral border, background surface, medium radius.
- **Focus:** visible ring and border emphasis, not a layout shift.
- **Error / Disabled:** semantic color or reduced opacity plus explicit text where needed.

### Navigation

- Full navigation belongs to authenticated app routes. The sidebar is appropriate for resource workflows because it carries cluster, namespace, and resource navigation. Standalone states like initialization and `/login` should not use the dashboard sidebar unless they truly allow app navigation.

### Operational Fault Page

- `/login` is a standalone operational fault page. Use the real Kite logo, a compact top bar, concise fault reason, refresh action, and operator checks for database, auth configuration, and backend logs. Do not show username/password inputs or OAuth provider buttons there.

## 6. Do's and Don'ts

### Do:

- **Do** keep operational pages dense enough for scanning and repeated action.
- **Do** use the real Kite logo asset when a standalone state needs brand identity.
- **Do** explain whether an error likely belongs to user permissions, auth configuration, database connectivity, or cluster access.
- **Do** keep table row heights, icon buttons, and toolbar controls stable across hover and loading states.

### Don't:

- **Don't** use marketing hero layouts, decorative gradients, glassmorphism, or identical card grids inside the app.
- **Don't** put a dashboard sidebar on a standalone error page just to make it feel integrated.
- **Don't** reintroduce a visible login form on `/login` without an explicit product decision.
- **Don't** use color alone to communicate severity or status.
