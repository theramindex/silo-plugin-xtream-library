# Silo Host Navigation and Sync Notes

## User Apps navigation

Silo's user sidebar has an Apps section for installed plugins. A plugin route appears there when the route descriptor is:

```json
{
  "method": "GET",
  "path": "/dispatcharr",
  "access": "authenticated",
  "navigable": true,
  "navigation_label": "Live TV",
  "navigation_kind": "user"
}
```

The Silo SDK v0.8.1 includes the required fields on `HttpRouteDescriptor`:

- `navigable`
- `navigation_label`
- `navigation_kind`

Use `navigation_kind: "user"` for normal user sidebar Apps entries. Use `navigation_kind: "admin"` with `access: "admin"` for admin navigation.

## Dispatcharr sync task

The plugin exposes three scheduled task capabilities:

```text
dispatcharr-sync
dispatcharr-refresh-channels
dispatcharr-refresh-epg
```

Silo registers that task as:

```text
plugin:<installation_id>:dispatcharr-sync
```

Current Silo host builds store task cadence outside of the plugin manifest. If no task binding trigger is configured, Silo falls back to a startup-only trigger. Configure full/channel refreshes at a slower cadence and EPG refreshes more frequently, for example:

```json
[
  { "type": "startup" },
  { "type": "interval", "interval_ms": 86400000 }
]
```

The startup trigger hydrates channels and EPG after install/restart. Silo owns
these trigger definitions; refresh-hour fields are intentionally not exposed in
the plugin config form.
