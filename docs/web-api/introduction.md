# Introduction

NeoRuntime exposes a unified **HTTP/JSON Web API** on every device. The same API backs the built-in web console and can be used directly from your own services, scripts or mobile apps to manage AI models, applications, media pipelines and hardware peripherals.

## Base URLs

The API is versioned under `/api/v1`. Which base URL you can reach depends on where your client runs:

| Base URL | Where it works | Notes |
| --- | --- | --- |
| `https://{host}/api/v1` | Anywhere (production) | nginx TLS gateway on port 443. The certificate is **self-signed** — disable certificate verification in your client (`curl -k`, `verify=False`, …). This is the only base reachable from remote machines. |
| `/api/v1` | Browser, same-origin | Resolved against wherever the page is served; no CORS involved. |
| `http://localhost:8080/api/v1` | On the device only | `platform-api` binds `127.0.0.1:8080`; the port is not reachable remotely. Useful for local debugging over SSH. |

WebSocket endpoints follow the same rule: use `wss://{host}/api/v1/...` remotely and `ws://localhost:8080/api/v1/...` on the device.

## Surface areas

The API is organised into the following functional groups (tags in the [API Reference](/api-reference/)):

- **System** — platform info, health, restart, OTA firmware updates, OS (A/B) upgrades, time/NTP configuration.
- **AI Runtime** — model registration/upload/parse, load/unload, per-model app usage, AI statistics.
- **Event Bus** — topics, publish, and a WebSocket subscription stream.
- **Device Control** — PTZ, zoom/focus/iris lens control with one-shot autofocus, GPIO, IR-CUT, white light/IR LED, fan, heater, radar, RS485, alarm/wiegand outputs, hardware capabilities.
- **App Manager** — install/start/stop/restart applications, permissions, logs, wizard installs, async package installs.
- **Stream** — zero-latency H264 video over WebSocket.
- **Media** — camera daemon configuration, ISP image parameters, encoder settings, OSD, privacy masks, pipeline profiles, per-stream add/remove, and audio capture/playback/talk.
- **Monitor** — CPU/memory/disk/network usage, one-shot snapshots, gyro attitude over SSE.
- **Processes / Files / Terminal / SSH / Logs** — process inspection and signalling, remote file management, interactive web terminal, sshd configuration, system and service logs (including a live WebSocket log stream).
- **Containers / Images** — containerd container and image lifecycle.
- **Storage** — disks, mount/unmount, format.
- **Event Logs / Debug Logs** — structured event history with localized (en/zh) templates, and tar.gz debug log export.
- **Network / Device Info / Settings / Store / Dev** — interface configuration, device naming and factory-programmed fields, dynamic key-value settings, application store, and the in-browser development workbench.

## Conventions

- All responses share one envelope: `{code, message, data, error}`. `code === 0` means success; anything else is a business error code — see [Errors & Status Codes](/errors).
- Except where documented otherwise (health check, public key, login, OTA status), every endpoint requires a Bearer token — see [Authentication](/authentication).
- Paths use `snake_case`; path parameters such as `{model_id}` are written with curly braces in this documentation and `:model_id` in gin source code.

## OpenAPI spec

This site renders the OpenAPI 3.0 spec that lives in the repository at
[`docs/api/swagger.yaml`](https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml).
A CI gate (`scripts/check_swagger_sync.py`) verifies that the spec documents
exactly the routes registered in `platform/platform-api/server/main.go`, so the
reference you are reading cannot drift from the code. See
[Keeping Docs in Sync](/update-mechanism) for how the site is rebuilt and how
translations are kept complete.
