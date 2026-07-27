# Feature Requirements Document

## Available Features

- [GWS-0001. Waybar config generation (config.jsonc)](features/GWS-0001-waybar-config-generation.md) - `#config-generation`
- [GWS-0002. Start menu XML generation (start.xml)](features/GWS-0002-start-menu-xml-generation.md) - `#config-generation` `#menu`
- [GWS-0003. Nested submenu support](features/GWS-0003-nested-submenu-support.md) - `#menu`
- [GWS-0004. Default config bootstrap](features/GWS-0004-default-config-bootstrap.md) - `#config` `#config-generation`
- [GWS-0005. Live config reload on file changes](features/GWS-0005-live-config-reload-on-file-changes.md) - `#config` `#reload`
- [GWS-0006. Waybar reload via SIGUSR2](features/GWS-0006-waybar-reload-via-sigusr2.md) - `#reload` `#process-lifecycle`
- [GWS-0007. Waybar cold start fallback](features/GWS-0007-waybar-cold-start-fallback.md) - `#reload` `#process-lifecycle`
- [GWS-0008. Single-instance enforcement with takeover](features/GWS-0008-single-instance-enforcement-with-takeover.md) - `#process-lifecycle`
- [GWS-0009. PID file tracking](features/GWS-0009-pid-file-tracking.md) - `#process-lifecycle`
- [GWS-0010. Daemon mode](features/GWS-0010-daemon-mode.md) - `#process-lifecycle` `#cli`
- [GWS-0011. Configurable logging](features/GWS-0011-configurable-logging.md) - `#logging` `#cli`
- [GWS-0012. Graceful shutdown on signals](features/GWS-0012-graceful-shutdown-on-signals.md) - `#process-lifecycle`
- [GWS-0013. Desktop notifications via D-Bus](features/GWS-0013-desktop-notifications.md) - `#logging`
- [GWS-0014. Configurable config path](features/GWS-0014-configurable-config-path.md) - `#config` `#cli`
- [GWS-0015. Multiple menu support across waybar modules](features/GWS-0015-multiple-menu-support.md) - `#config-generation` `#menu`
- [GWS-0016. Watch-only mode (--no-generate)](features/GWS-0016-watch-only-no-generate-mode.md) - `#config-generation` `#reload` `#cli`
- [GWS-0017. Fault-tolerant config reload](features/GWS-0017-fault-tolerant-reload.md) - `#config` `#reload` `#process-lifecycle`
- [GWS-0018. Backup of config.jsonc before regeneration (.bak)](features/GWS-0018-config-jsonc-backup.md) - `#config-generation` `#config`
- [GWS-0019. Config-file and environment-variable settings](features/GWS-0019-config-file-and-env-vars.md) - `#config` `#cli`
- [GWS-0020. Persist default sidecar settings to config file](features/GWS-0020-persist-default-settings.md) - `#config`
- [GWS-0021. Symlink, hardlink, and mount handling](features/GWS-0021-symlink-hardlink-mount-handling.md) - `#config` `#filesystem`
- [GWS-0022. Sweep stray waybar instances at startup](features/GWS-0022-sweep-stray-waybars-at-startup.md) - `#process-lifecycle` `#reload`
- [GWS-0023. Menu separator items and duplicate element IDs](features/GWS-0023-menu-separator-duplicate-ids.md) - `#menu` `#config-generation`
- [GWS-0024. Menu XML duplicate ID validation](features/GWS-0024-menu-xml-validation.md) - `#menu` `#config-generation`
- [GWS-0025. Cron-like jobs runner](features/GWS-0025-cronlike-jobs-runner.md) - `#config` `#jobs`
- [GWS-0026. Notification click actions](features/GWS-0026-notification-click-actions.md) - `#logging`
- [GWS-0027. Waybar process supervision](features/GWS-0027-waybar-process-supervision.md) - `#process-lifecycle` `#reload`

## Tags

- `#config`: GWS-0004, GWS-0005, GWS-0014, GWS-0017, GWS-0018, GWS-0019, GWS-0020, GWS-0021, GWS-0025
- `#config-generation`: GWS-0001, GWS-0002, GWS-0004, GWS-0015, GWS-0016, GWS-0018, GWS-0023, GWS-0024
- `#menu`: GWS-0002, GWS-0003, GWS-0015, GWS-0023, GWS-0024
- `#reload`: GWS-0005, GWS-0006, GWS-0007, GWS-0016, GWS-0017, GWS-0022, GWS-0027
- `#process-lifecycle`: GWS-0006, GWS-0007, GWS-0008, GWS-0009, GWS-0010, GWS-0012, GWS-0017, GWS-0022, GWS-0027
- `#logging`: GWS-0011, GWS-0013, GWS-0026
- `#cli`: GWS-0010, GWS-0011, GWS-0014, GWS-0016, GWS-0019
- `#filesystem`: GWS-0021
- `#jobs`: GWS-0025
