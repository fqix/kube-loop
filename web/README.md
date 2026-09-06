# KubeLoop browser applications

- `admin` — the Management Plane console; builds into the Control Plane's Go
  embedded assets.
- `auth` — the authentication page; also embedded by the Control Plane.
- `site` — the public website, built into `site/` for GitHub Pages.

These share the npm workspace defined at the repository root, so there is one
lockfile and one dependency installation. The desktop application is a sibling
workspace in [`desktop/`](../desktop).

Run from the repository root:

```bash
npm ci
npm run dev:admin
npm run dev:auth
npm run dev:site
npm run build          # every workspace, including the desktop type check
npm test
```

All three use React, Vite, Tailwind CSS 4, and shadcn conventions. GitHub Pages
rebuilds the website before deployment.
