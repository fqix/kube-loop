Release and IDE builds place platform-specific helper binaries here before
building the desktop backend:

- `kubeloop-helper[.exe]` — privileged service
- Windows uses the same `kubeloop-helper.exe` for service, install, and uninstall operations.

The desktop backend embeds them and materializes verified copies under
`~/.kubeloop/cache/components/<version>/<os>-<arch>/` (or the isolated
`~/.kubeloop-dev` tree for development builds).

Installing or replacing the helper always goes through the platform's
administrator authorization. Automatic TUN startup may reuse a healthy
development helper when no exact update was requested.
