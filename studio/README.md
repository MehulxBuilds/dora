# Dora Studio frontend

React + TypeScript + Vite. This frontend is a read-only local database explorer.

## Run Studio

From the repository root (stop the application owning the database first):

```sh
go run ./cli/cmd/dora studio
```

The CLI opens the browser at http://127.0.0.1:7432. No Node.js is needed for the embedded build.

## Frontend development

Start the CLI without opening a browser, then use Vite in another terminal:

```sh
# repository root
go run ./cli/cmd/dora studio --no-open

# separate terminal
cd studio
bun install --frozen-lockfile
bun run dev
```

Vite proxies /api to the local Go server on port 7432. Visit the URL printed by Vite. With npm instead, use npm install and npm run dev; the existing repository lockfile is Bun's.

## Build

```sh
cd studio
bun run build
# or npm run build if dependencies are already installed
```

The build runs TypeScript checks and produces dist/. This directory is tracked because assets.go embeds it into the Go executable. Rebuild before compiling Go after changing UI source. Never edit generated assets directly.

Features: search and namespace filtering, 50-row pages, typed value previews, JSON inspection, clickable references, index definitions, JSON scalar lookups, sequence/hash metadata, refresh and error/empty states. The overview currently loads all key summaries; this is intended for small embedded databases.

The installed command is `dora studio` (install from the root with `go install ./cli/cmd/dora`). It discovers databases in the current application's `dora/` directory. Override the directory with `dora studio --path /path/to/dora-dir`. Multiple databases appear in the sidebar selector; discovery includes nested `.db`, `.json`, `.dora`, and WAL-backed databases. Refresh reloads the selected database; restart Studio to discover new files. Explicit file paths remain supported.

## Studio dark interface

Studio always uses dark mode, including initial HTML and native control color scheme. The interface uses Tailwind utilities and shadcn controls, with self-hosted Geist Sans for every label and JSON value. No application CSS classes or mono fonts are used. The layout has a compact desktop namespace sidebar, mobile database/namespace selectors, consistent icon sizing, structured tables, and a keyboard-dismissable record inspector.

Database, tab, namespace, key search, pagination, index selection and record inspection are reflected in the URL. Studio handles empty/invalid API responses with connection guidance. In frontend development, start the Go backend on port 7432 (`go run ./cli/cmd/dora studio --no-open`) before Vite; Vite alone cannot read database files. Restart the Go CLI after rebuilding the embedded frontend.

Validation: `cd studio && npm run build`; `node --test --test-isolation=none tests/read-api.test.mjs` checks successful reads and malformed/offline API errors. Browser rendering and keyboard interaction still require an actual browser check.
