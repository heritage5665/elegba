# User Summary Worked Example

This example demonstrates a BFF-style aggregation endpoint that combines data
from three upstream services while forwarding the calling user's token.

## Run

```bash
docker compose up --build
```

Then request the endpoint with a bearer token:

```bash
curl -H 'Authorization: Bearer demo-token' \
  'http://localhost:8080/users/42/summary'
```

The response is:

```json
{
  "user_id": 42,
  "user_status": "ACTIVE",
  "user_onboarded_on": "2024-03-15T10:22:00Z",
  "account_balance": 1580.42,
  "account_has_pnd": true,
  "account_has_lien": true
}
```

## What happens

1. The incoming `Authorization` header is detached from the client request and
   forwarded to each upstream using the configured `client` auth mode.
2. The `user`, `ledger`, and `account` fetches run concurrently.
3. The user response is cached by user ID and a SHA-256 hash of the client token.
4. The transform step maps the upstream fields to the canonical response.
5. The account fetch can fail. In that case, the static fallback returns
   `has_pnd: false` and `has_lien: false` instead of failing the entire response.

## Cache isolation

The cache key is:

```text
user:{userId}:{sha256(Authorization)}
```

This prevents one user's data from being returned to another user requesting
the same user ID. The configuration loader rejects cache keys that merely
contain the literal text `sha256` without hashing the client token header.

## Failure modes

- Missing token: returns HTTP `401` with error code `CLIENT_AUTH_MISSING`.
- Account service unavailable: returns the configured fallback values.
- Token changed: the cache entry is different because the token hash changes.

The mock services are intentionally small and can be inspected in
[examples/mock/main.go](../../examples/mock/main.go).
