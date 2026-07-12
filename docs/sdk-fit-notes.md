# Silo SDK Fit Notes

| Primitive | SDK support | Evidence | Adaptation needed? |
|-----------|-------------|----------|--------------------|
| Plugin configuration persistence | Yes, with separate scopes | `global_config_schema`, `user_config_schema`, runtime `Configure` | Global connection/presentation settings arrive through `Configure`; browser preferences use Silo's per-user settings API. |
| Masked secret fields | Yes | `AdminFormField.secret`, `ADMIN_FORM_CONTROL_PASSWORD` in `common.proto` | No major adaptation. Use password control + secret field metadata in manifest schema. |
| Scheduled task execution | Yes | `scheduled_task.proto` | Three task keys dispatch full, channel-only, and guide-only work. Cadence remains host-owned. |
| Provider/source registration for one visible source named `Live TV` | Unclear / likely indirect | No first-party example in SDK; only general capability descriptors and runtime bootstrap are documented | Need implementation to adapt through the nearest Silo capability surface that backs the Jellyfin-compatible API. This is the biggest unknown to verify during bootstrap. |
| Playback/request path with fresh resolution at play time | Yes for plugin UI | `http_routes.proto` and manifest route descriptors | Stream routes resolve upstream targets at request time and redirect. Buffered SDK responses prevent backend media proxying. |
| Admin-visible connection / sync health reporting | Partial | `http_routes.proto`, general manifest metadata/config support | Likely surface status through plugin routes and/or capability output. Need local health model regardless of host UI affordances. |
| Manifest self-description / plugin bootstrap | Yes | `manifest.Load`, `runtime.Serve`, `CapabilityServers`, example embedded `manifest.json` | No adaptation beyond computing checksum and embedding manifest. |
| Jellyfin-compatible API exposure | Assumed host-provided integration surface, not explicit in SDK docs inspected | No explicit SDK example; product requirement from user | Plugin implementation must keep `Live TV` data aligned with whatever Silo ingests into its Jellyfin-compatible API layer. Treat this as a host integration constraint while wiring provider/routes. |

## Working conclusions

- The SDK is sufficient to bootstrap a real Go plugin with embedded manifest, runtime server, scheduled tasks, and HTTP routes.
- The largest unverified area is the exact Silo-side mechanism that turns plugin-provided data into a Jellyfin-visible `Live TV` source.
- Implementation should therefore keep the host-facing layer thin and isolate canonical models, cache, upstream clients, and sync logic so the final Silo adapter can change with minimal churn.
- HTTP route requests carry method/path/headers/body/query but no user identity.
  Backend payloads must stay user-neutral; per-user state belongs in Silo user config.
- Scheduling cadence is host-owned because the SDK exposes task execution, not
  declarative cron-style scheduling.
- HTTP routes/navigation are static manifest entries and cannot be toggled by
  runtime settings.

## Host architecture finding

- Silo's current host integration auto-registers `metadata_provider.v1` capabilities into the metadata provider system.
- That path is used for metadata search/enrichment on existing library/provider chains.
- The observed host code does **not** show a plugin capability that creates a new Jellyfin-visible Live TV catalog/source/channel model.
- Result: the Dispatcharr plugin can be aligned with config/runtime contracts today, but a true Jellyfin-facing Live TV source likely needs Silo host changes before the plugin can fulfill the original end-user goal by itself.
