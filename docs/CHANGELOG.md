# Changelog

# v1.1.0 (Unreleased)

## Added

- Fault-tolerant reload: missing YAML config no longer fails startup (GWS-0017).
- Backup of `config.jsonc` before modifying (GWS-0018).
- Config file and env variables, persisted defaults (GWS-0019, 0020).
- Symlink, hardlink and mount handling (GWS-0021).
- Sweep of stray Waybar processes at startup (GWS-0022).
- Menu separator duplicate ids and menu XML validation (GWS-0023, 0024).
- Cron-like jobs runner (GWS-0025).
- Notification click actions (GWS-0026).
- Waybar process supervision; sidecar fully manages Waybar state (GWS-0027).
- Generated application list, categorized and with icons (GWS-0028, 0031, 0032).
- Power profile menu (GWS-0029).
- Session actions menu (GWS-0030).
- GitHub Actions publishes Linux amd64/arm64 binaries: rolling `latest` from `main`, versioned releases from `v*` tags.
- Generated `config.jsonc` starts with `//` header: generation date, app name, version and source (GWS-0001). Version is set via `-ldflags -X main.version` (`make build`).

## Changed

- Notifications use D-Bus calls instead of `notify-send` (GWS-0013).
- Single instance takeover no longer relies on a PID file (GWS-0008).
- Config dir moved out of Waybar's; directories are watched by whitelist.
- Submenu indicator removed (GWS-0003).
- Unchanged-output check ignores the header, so a new timestamp alone does not rewrite the file or create a `.bak` (GWS-0018).
- Code deduplication and simplification.

## Fixed

- False positive "waybar not started" errors.
- XML menu dir is created if missing.

# v1.0.0

- Waybar config generation (GWS-0001).
- Start menu XML generation, nested submenus, multiple menus (GWS-0002, 0003, 0015).
- Default config bootstrap, configurable config path (GWS-0004, 0014).
- Live reload on file changes, Waybar reload via SIGUSR2, cold-start fallback (GWS-0005, 0006, 0007).
- Single instance enforcement with takeover, PID file tracking (GWS-0008, 0009).
- Daemon mode, configurable logging, graceful shutdown on signals (GWS-0010, 0011, 0012).
- Desktop notifications via `notify-send` (GWS-0013).
- Watch-only (no generate) mode (GWS-0016).
