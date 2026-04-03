# Kite Desktop

This directory contains the Electron desktop wrapper for Kite.

## Development

Run from repo root:

```bash
make desktop-dev
```

What it does:

1. Builds the frontend (`ui`) to `static/`
2. Builds the Go backend binary to `desktop/backend/`
3. Starts Electron and opens a native desktop window

## Packaging

```bash
make desktop-build
```

Installers are generated under `desktop/dist/`.

## Environment Overrides

- `KITE_DESKTOP_BACKEND`: absolute path to a backend binary (override auto-detected binary)
- `KITE_DESKTOP_PORT`: preferred backend start port (default: `18680`)
