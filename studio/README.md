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
