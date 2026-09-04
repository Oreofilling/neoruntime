# Quickstart

A five-minute tour: log in, call a protected endpoint, and subscribe to the event stream.

All examples use `curl` against a device reachable at `https://192.168.1.50` — adjust the host. Because the TLS certificate is self-signed, remote calls pass `-k`.

## 1. Log in and grab a token

```bash
curl -k -X POST https://192.168.1.50/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username": "admin", "password": "admin"}'
```

::: warning
The login route is `/api/login` — there is **no `/v1`** segment. The credentials above are the built-in defaults; change them via `POST /api/v1/system/password`.
:::

The response carries the token in the standard envelope:

```json
{
  "code": 0,
  "message": "Success",
  "data": { "token": "Bearer eyJhbGciOi...", "username": "admin" }
}
```

Note that `data.token` **already includes the `Bearer ` prefix** — put it in the `Authorization` header verbatim, do not prepend another `Bearer`.

## 2. Call a protected endpoint

```bash
TOKEN='Bearer eyJhbGciOi...'   # paste data.token here

curl -k https://192.168.1.50/api/v1/system/info \
  -H "Authorization: $TOKEN"
```

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "version": "1.0.2",
    "services": { "ai-runtime": true, "event-bus": true, "device-control": true, "app-manager": true }
  }
}
```

## 3. Upload and load an AI model

```bash
# upload + register
curl -k -X POST https://192.168.1.50/api/v1/ai/models/upload \
  -H "Authorization: $TOKEN" \
  -F 'model=@yolov8s.hef'

# load it into the AI runtime (use the id returned above)
curl -k -X POST https://192.168.1.50/api/v1/ai/models/<model_id>/load \
  -H "Authorization: $TOKEN"
```

## 4. Subscribe to the event stream (WebSocket)

The token is passed as a query parameter for WebSocket endpoints:

```javascript
const ws = new WebSocket(
  'wss://192.168.1.50/api/v1/events/stream?token=eyJhbGciOi...'
) //                                     ^ raw token, no "Bearer " prefix

ws.onmessage = (ev) => console.log(JSON.parse(ev.data))
```

::: tip
On the device itself you would use `ws://localhost:8080/api/v1/events/stream?token=...`.
:::

## 5. Next steps

- The full surface: [API Reference](/api-reference).
- How passwords can be RSA-encrypted, token lifetime and revocation: [Authentication](/authentication).
- Business error codes and the response envelope: [Errors & Status Codes](/errors).
