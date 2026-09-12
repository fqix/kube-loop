# ADR 0026: Remove the macOS helper Supervisor

- Status: Accepted
- Date: 2026-09-12
- Supersedes: ADR 0023

## Context

ADR 0023 added `kubeloop-supervisor`, a second root LaunchDaemon on macOS whose
only job was to replace `kubeloop-helper` without a further administrator
prompt once the Supervisor itself had been authorised. It carried its own
protocol (`internal/protocol/supervisor`), state machine, journal, rollback
logic, dev/release channel split and an `owner-trusted` development mode —
about 2.2k lines of darwin-only code plus a third embedded binary.

Its release phase (fixed Team ID / designated requirement, notarisation,
`SMAppService`) was never completed, so in practice it only served the
no-password developer and E2E iteration loop. The desktop shell has since
moved to Electron with electron-builder, which is the natural place to sign
and notarise the bundle; once the app is signed and the helper is registered
through `SMAppService`, macOS updates the daemon together with the app and the
Supervisor has no remaining purpose. The project chose to drop it now rather
than finish and then retire it.

## Decision

- Delete `cmd/kubeloop-supervisor`, `internal/supervisor`,
  `internal/supervisorapp`, `internal/protocol/supervisor` and the
  Supervisor-specific `helperinstall` paths. Only `kubeloop-helper` is embedded
  and installed on every platform.
- macOS uses the same direct install path as Linux: the helper's `install`
  subcommand runs once under `osascript … with administrator privileges` and
  replaces the binary, plist and auth files. A byte-different helper build
  therefore prompts for the administrator password again.
- The helper `install` and `uninstall` steps remove a leftover Supervisor
  LaunchDaemon (`dev.fengqi.kubeloop.supervisor[.dev]`), its binary and state
  files so machines that had ADR 0023 installs converge on the single-daemon
  layout without manual cleanup.

## Consequences

- One long-lived root process fewer, one embedded binary fewer, and no
  separate trust policy to maintain.
- Developer and E2E loops that rebuild the helper prompt for the administrator
  password on each byte-distinct build. The existing "reuse a healthy,
  protocol-compatible dev helper" behaviour in `helperinstall` still avoids the
  prompt when the helper source did not change.
- Signing, notarisation and `SMAppService` registration for release builds are
  now tracked against the Electron packaging pipeline (electron-builder
  `mac.identity` / `afterSign`), not against a Supervisor phase.
