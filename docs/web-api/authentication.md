# Authentication

The API uses **JWT Bearer tokens**. One endpoint issues tokens; everything else (except the public endpoints listed below) expects the token in the `Authorization` header.

## Logging in

`POST /api/login` — note the full path: the login route lives outside the `/api/v1` group and has **no `/v1` segment**.

```json
{
  "username": "admin",
  "password": "<password>",
  "timestamp": 1725400000
}
```

- `username` / `password` — required. When no credentials are configured on the device, the built-in default `admin` / `admin` applies.
- `timestamp` — optional Unix seconds for replay protection. When non-zero, the request is rejected if the clock skew exceeds the freshness window (`code 1005`). Omit or send `0` to skip the check (legacy behaviour).

Success returns the envelope with `data.token` — a JWT that **already carries the `Bearer ` prefix**:

```json
{ "code": 0, "message": "Success", "data": { "token": "Bearer eyJhbGciOi...", "username": "admin" } }
```

Invalid credentials return HTTP 401 with `code 2000`.

## Using the token

Send the token verbatim in the `Authorization` header — do **not** add a second `Bearer`:

```bash
curl -k https://HOST/api/v1/system/stats -H "Authorization: Bearer eyJhbGciOi..."
```

For **WebSocket** and **SSE** endpoints, headers cannot always be set (`EventSource` cannot set custom headers at all), so the raw token — **without** the `Bearer ` prefix — is passed as the `token` query parameter:

```
wss://HOST/api/v1/events/stream?token=eyJhbGciOi...
```

## Encrypting the password (optional, recommended)

Instead of sending the password in plaintext over TLS, a client can encrypt it with the device's RSA public key:

1. `GET /api/v1/auth/public-key` (no authentication) returns a PEM public key.
2. Encrypt the password with **RSA-2048 / PKCS#1 v1.5** and base64-encode the ciphertext.
3. Put the base64 ciphertext in the `password` field; set `timestamp` to the current Unix time so a captured ciphertext cannot be replayed.

Decryption on the device is lenient: a value that cannot be decrypted is treated as plaintext, so legacy clients keep working during a rollout.

## Token lifetime and revocation

- The JWT signing secret is **regenerated at every boot** — after a device restart all previously issued tokens are invalid; simply log in again.
- `POST /api/v1/logout` revokes the token used for the request **server-side** (the token is added to a revocation list; it stops working immediately, not just at expiry).

## Endpoints that need no token

| Endpoint | Why it is public |
| --- | --- |
| `GET /api/v1/system/health` | Liveness probes. |
| `GET /api/v1/auth/public-key` | Fetched before a session token exists. |
| `POST /api/login` | Issues the token itself. |
| `GET /api/v1/system/ota/status` | Install progress must remain pollable from a fresh client while an upgrade is in flight. |
| `GET /api/v1/system/os-upgrade/status` | Same rationale as OTA status. |

A missing, invalid or revoked token on any other endpoint returns HTTP 401 with business code `2000` (or `2002` expired / `2003` invalid token where distinguishable).
