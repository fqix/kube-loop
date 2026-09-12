# KubeLoop web monorepo

This directory is an npm workspaces monorepo with one lockfile and one
dependency installation:

- `apps/admin` — the browser admin console; builds to the Control Plane's Go
  embed assets.

Run commands from the repository root:

```bash
npm ci --prefix frontend
npm run dev:admin --prefix frontend
npm run build --prefix frontend
npm test --prefix frontend
```
