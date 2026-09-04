---
title: "Errors & Status Codes"
---

## The response envelope

Every endpoint answers with the same JSON envelope:

```json
{
  "code": 0,               // business code — 0 means success
  "message": "Success",    // human-readable summary
  "data": { },             // payload on success (absent or null on failure)
  "error": {               // optional, on failure
    "type": "validation",  // category: validation | auth | service | resource | system
    "detail": "..."        // specifics of what went wrong
  }
}
```

Always branch on `code`, not on `message` — messages are for humans and may change; codes are the contract.

## HTTP status vs business code

The HTTP status is a coarse signal; the business `code` is the precise one:

| HTTP | Typical business codes | Meaning |
| --- | --- | --- |
| 200 | `0` | Success. |
| 400 | `1001` `1002` `1003` `1004` `1005` | Validation failures — malformed JSON, missing/invalid parameters, stale timestamp. |
| 401 | `2000` `2002` `2003` | Missing, invalid, expired or revoked Bearer token; bad credentials at login. |
| 403 | `2001` | Authenticated but not allowed. |
| 404 | `4000` `5000` `6000` `8000` `10000` | A referenced resource (app, model, file, process…) does not exist. |
| 409 | `4001` `6004` `6005` | Conflict — already exists, or app state does not allow the operation. |
| 5xx | `3000`–`3004`, others | Service unavailable/timeout, backend gRPC or database errors. |

## Business code registry

Codes are grouped by thousand-bands; the groups are stable, individual codes are additive.

| Band | Group | Codes |
| --- | --- | --- |
| `0` | Success | `0` Success |
| `1xxx` | General / request errors | `1000` unknown · `1001` invalid request · `1002` invalid JSON · `1003` missing parameter · `1004` invalid parameter · `1005` timestamp outside freshness window |
| `2xxx` | Authentication & authorization | `2000` unauthorized · `2001` forbidden · `2002` token expired · `2003` invalid token |
| `3xxx` | Service / infrastructure | `3000` service unavailable · `3001` service timeout · `3002` service error · `3003` gRPC error · `3004` database error |
| `4xxx` | Resource errors | `4000` not found · `4001` already exists · `4002` resource exhausted · `4003` operation failed |
| `5xxx` | AI / model errors | `5000` model not found · `5001` model load failed · `5002` inference error · `5003` invalid model format |
| `6xxx` | App manager errors | `6000` app not found · `6001` install failed · `6002` start failed · `6003` stop failed · `6004` app already running · `6005` app not running |
| `7xxx` | Device errors | `7000` device error · `7001` PTZ error · `7002` camera error · `7003` GPIO error |
| `8xxx` | File / storage errors | `8000` file not found · `8001` upload failed · `8002` delete failed · `8003` storage full · `8004` access denied |
| `9xxx` | SSH errors | `9000` config error · `9001` service error |
| `10xxx` | Process errors | `10000` process not found · `10001` kill failed |

The authoritative list lives in `platform/platform-api/handlers/response.go`; the per-operation documentation in the [API Reference]((/neoruntime/api-reference/)) spells out the success responses, and the 400/401 behaviour described here applies to every write endpoint even where not repeated per operation.

## Example: validation failure

```bash
curl -k -X POST https://HOST/api/v1/ai/models \
  -H "Authorization: Bearer eyJ..." \
  -H 'Content-Type: application/json' \
  -d '{}'
```

```json
{
  "code": 1003,
  "message": "Missing required parameter",
  "error": { "type": "validation", "detail": "path is required" }
}
```

## Example: expired token

```json
{
  "code": 2002,
  "message": "Token has expired",
  "error": { "type": "auth", "detail": "re-login required" }
}
```
