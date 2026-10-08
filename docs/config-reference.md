# Elegba — Configuration Reference

**Version:** 1.1
**Schema Version:** `"1"`
**Applies to:** Elegba v1.x

This document is the exhaustive reference for Elegba's configuration file.
Every field, its type, default value, and behavior is documented here.

---

## Table of Contents

1. [File Format](#1-file-format)
2. [Environment Variable Interpolation](#2-environment-variable-interpolation)
3. [Top-Level Structure](#3-top-level-structure)
4. [`version`](#4-version)
5. [`server`](#5-server)
6. [`upstreams`](#6-upstreams)
7. [`caches`](#7-caches)
8. [`endpoints`](#8-endpoints)
9. [Step Types](#9-step-types)
10. [Template Functions](#10-template-functions)
11. [Validation Rules](#11-validation-rules)
12. [Hot Reload](#12-hot-reload)
13. [Complete Example](#13-complete-example)
14. [Migration Guide](#14-migration-guide)

---

## 1. File Format

Elegba accepts configuration in **YAML** (primary, recommended) or **JSON**.

- **YAML**: parsed with `gopkg.in/yaml.v3`, strict mode enabled
  (`KnownFields(true)`). Unknown keys are rejected.
- **JSON**: parsed with `encoding/json`,
  `Decoder.DisallowUnknownFields()` enabled. Unknown keys are rejected.

The file extension determines the parser:
- `.yaml`, `.yml` → YAML parser
- `.json` → JSON parser

**Encoding**: UTF-8 only. BOM is rejected.

**File size limit**: 10 MB (configurable via `ELEGBA_CONFIG_MAX_BYTES`).

---

## 2. Environment Variable Interpolation

Elegba supports environment variable interpolation anywhere a string value
appears in the config.

### Syntax

| Syntax | Behavior |
|--------|----------|
| `${VAR}` | Required. Fails at load time if `VAR` is unset. |
| `${VAR:-default}` | Optional. Uses `default` if `VAR` is unset or empty. |
| `${VAR:?error message}` | Required with custom error. Fails with `error message` if unset. |
| `$${VAR}` | Escape. Produces the literal string `${VAR}`. |

### Rules

- Interpolation happens **after** YAML/JSON parsing and **before** validation.
- Interpolation applies to **string values only**. Numbers, booleans, and
  durations are not interpolated — wrap them in quotes if you need
  interpolation.
- Nested interpolation is not supported (`${A${B}}` is invalid).
- Empty defaults are allowed: `${VAR:-}` yields an empty string.
- Interpolation is single-pass — the result is not re-scanned.

### Examples

```yaml
upstreams:
  user-service:
    baseURL: ${USER_SERVICE_URL}
    auth:
      type: bearer
      token: ${USER_TOKEN:?USER_TOKEN is required}
    timeout: ${USER_TIMEOUT:-2s}
```

### Reserved Variables

The following environment variables are read by Elegba itself and are not
part of interpolation:

| Variable | Purpose | Default |
|----------|---------|---------|
| `ELEGBA_CONFIG` | Path to the config file. | `elegba.yaml` |
| `ELEGBA_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`). | `info` |
| `ELEGBA_LOG_FORMAT` | Log format (`json`, `text`). | `json` |
| `ELEGBA_ADMIN_ADDR` | Admin server address (overrides config). | — |
| `ELEGBA_CONFIG_MAX_BYTES` | Max config file size. | `10485760` (10 MB) |
| `ELEGBA_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout. | `30s` |
| `ELEGBA_HOT_RELOAD` | Enable hot reload. | `true` |
| `ELEGBA_HOT_RELOAD_DEBOUNCE` | Debounce interval. | `500ms` |
| `ELEGBA_CACHE_BACKEND` | `ristretto` or `bigcache`. | `ristretto` |
| `ELEGBA_OTEL_ENDPOINT` | OTLP endpoint. | — |
| `ELEGBA_OTEL_INSECURE` | Use insecure OTLP. | `false` |

---

## 3. Top-Level Structure

```yaml
version: "1"

server:
  # see §5

upstreams:
  # see §6

caches:
  # see §7

endpoints:
  # see §8
```

All four sections are optional at the schema level, but Elegba refuses to
start if there are no `endpoints` defined (nothing to serve) or if an endpoint
references an upstream that is not defined.

---

## 4. `version`

| Field | Type | Required | Default |
|-------|------|----------|---------|
| `version` | string | yes | — |

The configuration schema version. Must be a string matching a known version.

**Supported values:**
- `"1"` — current schema.

**Behavior:**
- Unknown versions cause startup to fail with a clear error.
- The version must be a **string**, not a number (`"1"`, not `1`).

```yaml
version: "1"
```

---

## 5. `server`

Configuration for the HTTP server that Elegba exposes to clients.

```yaml
server:
  address: ":8080"
  readTimeout: 5s
  writeTimeout: 10s
  idleTimeout: 120s
  readHeaderTimeout: 5s
  shutdownTimeout: 30s
  maxBodyBytes: 1048576
  tls:
    certFile: /etc/elegba/tls/cert.pem
    keyFile: /etc/elegba/tls/key.pem
  cors:
    allowedOrigins: ["https://app.example.com"]
    allowedMethods: ["GET", "POST"]
    allowedHeaders: ["Authorization", "Content-Type"]
    exposedHeaders: ["X-Request-ID"]
    allowCredentials: true
    maxAge: 600s
  admin:
    address: ":9090"
    enabled: false
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | no | `":8080"` | Host:port to listen on. |
| `readTimeout` | duration | no | `5s` | Max time to read the full request, including body. |
| `writeTimeout` | duration | no | `10s` | Max time to write the response. |
| `idleTimeout` | duration | no | `120s` | Max time to keep idle keep-alive connections. |
| `readHeaderTimeout` | duration | no | `5s` | Max time to read request headers. Protects against slowloris. |
| `shutdownTimeout` | duration | no | `30s` | Max time to drain in-flight requests on shutdown. |
| `maxBodyBytes` | int64 | no | `1048576` (1 MB) | Max incoming request body size. |
| `tls` | object | no | — | Server-side TLS config (§5.1). |
| `cors` | object | no | — | CORS config (§5.2). |
| `admin` | object | no | — | Admin server config (§5.3). |

### 5.1 `server.tls`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `certFile` | string | yes | — | Path to PEM-encoded certificate. |
| `keyFile` | string | yes | — | Path to PEM-encoded private key. |
| `minVersion` | string | no | `"1.2"` | Minimum TLS version (`"1.2"`, `"1.3"`). |
| `clientCAFile` | string | no | — | Path to CA bundle for client cert verification (mTLS). |

If `tls` is omitted, the server listens on plain HTTP. This is fine behind a
TLS-terminating proxy but should be avoided for direct internet exposure.

### 5.2 `server.cors`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `allowedOrigins` | []string | no | `[]` | Origins allowed. Use `["*"]` for any (never with credentials). |
| `allowedMethods` | []string | no | `["GET", "POST", "PUT", "DELETE", "OPTIONS"]` | Allowed HTTP methods. |
| `allowedHeaders` | []string | no | `["Content-Type", "Authorization"]` | Request headers allowed. |
| `exposedHeaders` | []string | no | `[]` | Response headers exposed to the browser. |
| `allowCredentials` | bool | no | `false` | Whether to allow cookies/auth headers. |
| `maxAge` | duration | no | `0s` | How long preflight responses are cached. |

**Safety rule**: If `allowCredentials: true`, `allowedOrigins` must not
contain `"*"`. Elegba fails at load time if both are set.

### 5.3 `server.admin`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | no | `":9090"` | Host:port for the admin server. |
| `enabled` | bool | no | `false` | Enable the admin server. |

The admin server exposes:
- `/metrics` — Prometheus metrics
- `/debug/pprof/*` — Go pprof endpoints (if `enabled: true`)
- `/healthz` — liveness (also on the main server)
- `/readyz` — readiness (also on the main server)

**Security note**: Bind the admin server to a private interface or protect it
with network policy. Do not expose it publicly.

---

## 6. `upstreams`

Named upstream API clients. Each upstream is a fully configured HTTP (or
future gRPC) client with its own transport, auth, resilience, and connection
pool.

```yaml
upstreams:
  user-service:
    transport: http
    baseURL: https://api.example.com
    timeout: 2s
    maxResponseBytes: 10485760
    maxConcurrent: 100
    connectionPool:
      maxIdleConns: 100
      maxIdleConnsPerHost: 100
      idleConnTimeout: 90s
    retries: 2
    retry:
      initialInterval: 100ms
      maxInterval: 2s
      multiplier: 2.0
      maxElapsedTime: 10s
      jitter: true
    breaker:
      maxRequests: 5
      interval: 60s
      timeout: 30s
      failureThreshold: 5
    rateLimit: 100/s
    auth:
      type: client
      header: Authorization
      forward: Authorization
      scheme: Bearer
    tls:
      caFile: /etc/elegba/ca.pem
      certFile: /etc/elegba/cert.pem
      keyFile: /etc/elegba/key.pem
      insecureSkipVerify: false
    headers:
      X-Client: elegba
    proxy:
      url: http://proxy.internal:3128
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `transport` | string | no | `"http"` | Transport type. Currently only `"http"`. |
| `baseURL` | string | yes | — | Base URL of the upstream. Must be absolute. |
| `timeout` | duration | no | `2s` | Per-request timeout. |
| `maxResponseBytes` | int64 | no | `10485760` (10 MB) | Max response body size. |
| `maxConcurrent` | int | no | `100` | Max concurrent requests. |
| `connectionPool` | object | no | defaults below | Connection pooling settings for this upstream's HTTP transport. |
| `retries` | int | no | `2` | Number of retry attempts (in addition to the first). |
| `retry` | object | no | — | Retry policy (§6.1). |
| `breaker` | object | no | — | Circuit breaker config (§6.2). |
| `rateLimit` | string | no | — | Rate limit as `"N/duration"` (e.g., `"100/s"`). |
| `auth` | object | no | — | Auth config (§6.3). |
| `tls` | object | no | — | Per-upstream TLS config (§6.4). |
| `headers` | map[string]string | no | `{}` | Static headers added to every request. |
| `proxy` | object | no | — | Proxy config (§6.5). |

### 6.1 `upstreams.*.retry`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `initialInterval` | duration | no | `100ms` | First backoff interval. |
| `maxInterval` | duration | no | `2s` | Maximum backoff interval. |
| `multiplier` | float | no | `2.0` | Backoff multiplier. |
| `maxElapsedTime` | duration | no | `10s` | Total time budget for retries. |
| `jitter` | bool | no | `true` | Add randomization to backoff. |

**Retry policy**:
- Retries only for idempotent methods (GET, HEAD, OPTIONS) by default.
- Retries on network errors, 5xx, 429 (respects `Retry-After`).
- Does NOT retry on 4xx (except 429).

### 6.2 `upstreams.*.breaker`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `maxRequests` | int | no | `5` | Max requests allowed in half-open state. |
| `interval` | duration | no | `60s` | Cyclic period for clearing counts in closed state. |
| `timeout` | duration | no | `30s` | How long to stay in open state before half-open. |
| `failureThreshold` | int | no | `5` | Consecutive failures to trip the breaker. |

### 6.3 `upstreams.*.auth`

Supported `type` values: `bearer`, `basic`, `apikey`, `oauth2`, `client`.

#### `type: bearer`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string | yes | Bearer token. Supports `${VAR}`. |

#### `type: basic`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | yes | Username. |
| `password` | string | yes | Password. Supports `${VAR}`. |

#### `type: apikey`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | yes | API key value. |
| `in` | string | no | Where to put the key: `"header"` or `"query"`. Default `"header"`. |
| `name` | string | no | Header or query parameter name. Default `"X-API-Key"`. |

#### `type: oauth2`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tokenURL` | string | yes | OAuth2 token endpoint. |
| `clientID` | string | yes | Client ID. |
| `clientSecret` | string | yes | Client secret. Supports `${VAR}`. |
| `scopes` | []string | no | Requested scopes. |
| `audience` | string | no | Audience (for Auth0-style providers). |

Tokens are cached and refreshed automatically before expiry.

#### `type: client`

Forwards the client's token to the upstream. This is the standard pattern for
BFFs that act on behalf of the calling user.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `header` | string | no | `"Authorization"` | Incoming header to read the token from. |
| `forward` | string | no | same as `header` | Outgoing header to send. |
| `scheme` | string | no | `"Bearer"` | Scheme prefix to apply. Set to `""` to forward verbatim. |

**Behavior:**

1. Extract `.header.<header>` from the incoming request.
2. Strip the existing scheme prefix if present (e.g., `Bearer `).
3. Re-add the configured `scheme` prefix.
4. Set the `forward` header on the upstream request.
5. If the client's header is missing → return `401 CLIENT_AUTH_MISSING`.

**Example:**

```yaml
upstreams:
  user-service:
    baseURL: https://users.internal
    auth:
      type: client
      header: Authorization
      forward: Authorization
      scheme: Bearer
```

**Security:**

- Tokens are never logged; they are redacted in structured logs.
- Cache keys MUST include a token hash when `auth.type: client` is used.
  Elegba enforces this at config load time.
- TLS MUST be enforced between Elegba and the upstream.
- Do not forward a token to an upstream that should not see it.

### 6.4 `upstreams.*.tls`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `caFile` | string | no | — | Path to CA bundle. |
| `certFile` | string | no | — | Path to client cert (mTLS). |
| `keyFile` | string | no | — | Path to client key (mTLS). |
| `insecureSkipVerify` | bool | no | `false` | Skip server cert verification. **Not recommended.** |
| `serverName` | string | no | — | Override SNI server name. |

### 6.5 `upstreams.*.proxy`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | Proxy URL (e.g., `http://proxy:3128`). |

---

## 7. `caches`

Named cache backends. Each cache is a fully configured cache client.

```yaml
caches:
  memory:
    type: in-memory
    maxSize: 10000
    defaultTTL: 60s
  redis:
    type: redis
    address: localhost:6379
    password: ${REDIS_PASSWORD}
    db: 0
    defaultTTL: 300s
    poolSize: 10
```

### Fields (common)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `type` | string | yes | — | Cache type: `"in-memory"` or `"redis"`. |
| `defaultTTL` | duration | no | `60s` | Default TTL when a step doesn't specify one. |

### 7.1 `type: in-memory`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `maxSize` | int | no | `10000` | Max number of entries. |
| `maxCost` | int64 | no | `0` | Max cost (bytes) if using ristretto. `0` = unlimited. |

Backed by `ristretto` (default) or `bigcache` (set `ELEGBA_CACHE_BACKEND=bigcache`).

### 7.2 `type: redis`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | yes | — | Redis address (host:port). |
| `password` | string | no | — | Redis password. Supports `${VAR}`. |
| `db` | int | no | `0` | Redis database number. |
| `poolSize` | int | no | `10` | Connection pool size. |
| `maxRetries` | int | no | `3` | Max retries on Redis errors. |
| `dialTimeout` | duration | no | `5s` | Connection dial timeout. |
| `readTimeout` | duration | no | `3s` | Read timeout. |
| `writeTimeout` | duration | no | `3s` | Write timeout. |
| `tls` | object | no | — | Redis TLS config (same shape as §6.4). |

---

## 8. `endpoints`

Incoming HTTP routes and their pipelines. Each endpoint defines a path, method,
and a list of steps to execute.

```yaml
endpoints:
  - path: /dashboard
    method: GET
    timeout: 10s
    maxConcurrent: 100
    failFast: false
    cacheInvalidate:
      - "user:{{ .query.userId }}"
    pipeline:
      - id: user
        type: fetch
        upstream: user-service
        path: /users/{{ .query.userId }}
        method: GET
        headers:
          X-Trace: "{{ .requestID }}"
        cache:
          backend: memory
          ttl: 60s
          key: "user:{{ .query.userId }}:{{ .header.Authorization | sha256 }}"
          staleWhileRevalidate: true
      - id: orders
        type: fetch
        upstream: order-service
        path: /orders?userId={{ .query.userId }}
      - id: combined
        type: transform
        dependsOn: [user, orders]
        template: |
          {{ dict
              "user" .user
              "orderCount" (len .orders)
            | toJSON }}
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `path` | string | yes | — | URL path. Supports `{param}` segments. |
| `method` | string | yes | — | HTTP method (`GET`, `POST`, `PUT`, `DELETE`, `PATCH`). |
| `timeout` | duration | no | `10s` | Per-request timeout for the whole pipeline. |
| `maxConcurrent` | int | no | `100` | Max concurrent requests for this endpoint. |
| `failFast` | bool | no | `false` | If `true`, abort on first step failure. |
| `cacheInvalidate` | []string | no | `[]` | Cache keys to invalidate before pipeline runs. |
| `pipeline` | []Step | yes | — | Ordered list of steps (see §9). |

**Path parameters**: segments like `/users/{id}` are extracted into
`{{ .path.id }}` in templates.

**Query parameters**: available as `{{ .query.paramName }}`.

**Headers**: available as `{{ .header.HeaderName }}`.

**Body**: available as `{{ .body }}` (parsed JSON) or `{{ .bodyRaw }}` (raw
string).

**Request ID**: available as `{{ .requestID }}`.

---

## 9. Step Types

Each step has a `type` field that determines its behavior. All steps share
these fields:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | string | yes | — | Unique step ID within the pipeline. |
| `type` | string | yes | — | Step type: `fetch`, `transform`, `cache`. |
| `dependsOn` | []string | no | `[]` | Step IDs this step depends on. |
| `onError` | string | no | `"fail"` | Error handling: `fail`, `ignore`, `fallback`. |
| `fallback` | object | no | — | Fallback config (if `onError: fallback`). |

### 9.1 `type: fetch`

Calls an upstream API.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `upstream` | string | yes | — | Name of the upstream to call. |
| `path` | string | yes | — | Path template (supports Go template syntax). |
| `method` | string | no | `"GET"` | HTTP method. |
| `headers` | map[string]string | no | `{}` | Additional headers (templated). |
| `query` | map[string]string | no | `{}` | Additional query params (templated). |
| `body` | string | no | — | Request body (templated). |
| `cache` | object | no | — | Cache config for this step (§9.1.1). |

#### 9.1.1 `fetch.cache`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `backend` | string | yes | — | Cache name from `caches`. |
| `key` | string | yes | — | Cache key template. |
| `ttl` | duration | no | backend default | TTL for this entry. |
| `staleWhileRevalidate` | bool | no | `false` | Serve stale while refreshing. |

**Cache safety rule**: When the upstream uses `auth.type: client`, the cache
key MUST include a hash of the client token (e.g.,
`{{ .header.Authorization | sha256 }}`). Elegba fails at config load time if
this rule is violated.

### 9.2 `type: transform`

Renders a Go template using previous step outputs.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `template` | string | yes | Go template. See §10 for functions. |

The template receives all prior step outputs as top-level variables keyed by
step ID.

### 9.3 `type: cache`

Explicit cache read or write (for use outside fetch steps).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | yes | `"get"` or `"set"`. |
| `backend` | string | yes | Cache name from `caches`. |
| `key` | string | yes | Cache key template. |
| `value` | string | no (required for `set`) | Value template. |
| `ttl` | duration | no | TTL for `set`. |

### 9.4 `onError` Behavior

- `fail` (default): step failure aborts the pipeline if `failFast: true`,
  otherwise records the error and continues.
- `ignore`: step failure is silently ignored; the step's output is `null`.
- `fallback`: step failure triggers the `fallback` config.

#### `fallback`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `static` | string | no | Static JSON to return. |
| `upstream` | string | no | Alternate upstream name. |
| `cacheKey` | string | no | Alternate cache key. |

Only one of `static`, `upstream`, or `cacheKey` may be set.

---

## 10. Template Functions

Templates use Go's `text/template` syntax with the following custom functions.

### Data Access

| Function | Description | Example |
|----------|-------------|---------|
| `.stepID` | Output of a previous step. | `{{ .user }}` |
| `.query.name` | Query parameter. | `{{ .query.userId }}` |
| `.path.name` | Path parameter. | `{{ .path.id }}` |
| `.header.Name` | Request header. | `{{ .header.Authorization }}` |
| `.body` | Parsed JSON body. | `{{ .body.email }}` |
| `.bodyRaw` | Raw body string. | `{{ .bodyRaw }}` |
| `.requestID` | Request ID (ULID/UUIDv7). | `{{ .requestID }}` |

### Encoding

| Function | Description | Example |
|----------|-------------|---------|
| `toJSON` | Marshal to JSON. | `{{ .user \| toJSON }}` |
| `fromJSON` | Unmarshal JSON string. | `{{ fromJSON .raw }}` |
| `jsonPath` | Extract via JSONPath. | `{{ jsonPath .user "$.name" }}` |
| `toBase64` | Base64 encode. | `{{ .data \| toBase64 }}` |
| `fromBase64` | Base64 decode. | `{{ .encoded \| fromBase64 }}` |
| `urlEncode` | URL-encode a string. | `{{ .q \| urlEncode }}` |
| `sha256` | Hex-encoded SHA-256. | `{{ .token \| sha256 }}` |

### Collections

| Function | Description | Example |
|----------|-------------|---------|
| `len` | Length of slice, map, or string. | `{{ len .orders }}` |
| `index` | Index into slice or map. | `{{ index .orders 0 }}` |
| `first` | First element. | `{{ first .orders }}` |
| `last` | Last element. | `{{ last .orders }}` |
| `sort` | Sort a slice. | `{{ sort .ids }}` |
| `unique` | Deduplicate a slice. | `{{ unique .ids }}` |
| `filter` | Filter a slice by field. | `{{ filter .orders "status" "paid" }}` |
| `map` | Map a field from each element. | `{{ map .orders "id" }}` |
| `dict` | Build a map from key-value pairs. | `{{ dict "a" 1 "b" 2 }}` |

### Math

| Function | Description | Example |
|----------|-------------|---------|
| `add` | Addition. | `{{ add 1 2 }}` |
| `sub` | Subtraction. | `{{ sub 5 3 }}` |
| `mul` | Multiplication. | `{{ mul 2 3 }}` |
| `div` | Division. | `{{ div 10 2 }}` |
| `mod` | Modulo. | `{{ mod 10 3 }}` |
| `sum` | Sum a slice. | `{{ sum .orders "amount" }}` |

### Strings

| Function | Description | Example |
|----------|-------------|---------|
| `upper` | Uppercase. | `{{ upper .name }}` |
| `lower` | Lowercase. | `{{ lower .name }}` |
| `trim` | Trim whitespace. | `{{ trim .name }}` |
| `replace` | Replace substring. | `{{ replace .s "a" "b" }}` |
| `replacePrefix` | Remove a prefix. | `{{ replacePrefix .s "Bearer " "" }}` |
| `split` | Split string. | `{{ split .csv "," }}` |
| `join` | Join slice. | `{{ join .ids "," }}` |
| `contains` | Substring check. | `{{ contains .s "foo" }}` |
| `redact` | Redact a value for logging. | `{{ redact .token }}` |

### Time

| Function | Description | Example |
|----------|-------------|---------|
| `now` | Current time (RFC3339). | `{{ now }}` |
| `formatTime` | Format a time. | `{{ formatTime .ts "2006-01-02" }}` |
| `unix` | Unix timestamp. | `{{ unix }}` |

### Misc

| Function | Description | Example |
|----------|-------------|---------|
| `default` | Default if nil/empty. | `{{ default "N/A" .name }}` |
| `coalesce` | First non-empty. | `{{ coalesce .a .b .c }}` |
| `uuid` | Generate UUIDv7. | `{{ uuid }}` |
| `env` | Read env var. | `{{ env "HOSTNAME" }}` |

**Security note**: `env` reads environment variables at template execution
time. Use only with trusted config.

---

## 11. Validation Rules

Elegba validates the config at load time and fails fast with a clear error if
any rule is violated.

### Structural

- `version` must be a supported string.
- Unknown top-level keys are rejected.
- Unknown keys within any section are rejected.

### Server

- `server.address` must be a valid `host:port`.
- Durations must parse with `time.ParseDuration`.
- `server.tls.certFile` and `server.tls.keyFile` must be readable if `tls` is
  set.
- If `server.cors.allowCredentials: true`, `allowedOrigins` must not contain
  `"*"`.

### Upstreams

- Every upstream name must be unique.
- `baseURL` must be a valid absolute URL with scheme `http` or `https`.
- `transport` must be a known type.
- `auth.type` must be a known type (`bearer`, `basic`, `apikey`, `oauth2`,
  `client`).
- `rateLimit` must match `N/duration` format.
- If `auth.type: oauth2`, `tokenURL` must be a valid URL.
- If `auth.type: client` and the upstream is used by a cached `fetch` step,
  the cache key must contain a token hash. See §9.1.1.

### Caches

- Every cache name must be unique.
- `type` must be `"in-memory"` or `"redis"`.
- If `type: redis`, `address` must be set.

### Endpoints

- `path` must start with `/`.
- `method` must be a valid HTTP method.
- Every `pipeline[*].id` must be unique within the endpoint.
- Every `dependsOn` reference must point to an existing step ID in the same
  pipeline.
- The dependency graph must be acyclic (no cycles).
- Every `fetch.upstream` must reference a defined upstream.
- Every `cache.backend` must reference a defined cache.
- Templates must parse successfully.
- `onError: fallback` requires a `fallback` object.

### Environment

- All required `${VAR}` references must resolve at load time.
- `${VAR:?message}` failures produce the custom message.

---

## 12. Hot Reload

Elegba watches the config file for changes and reloads atomically.

### Behavior

1. On file change, the watcher reads the file.
2. The new config is parsed and validated.
3. If invalid, the error is logged and the **old config continues running**.
4. If valid, the new engine is built.
5. The new engine is swapped in via `atomic.Pointer[Engine]`.
6. In-flight requests from the old engine are allowed to complete.
7. The old engine is closed after `shutdownTimeout`.

### Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `ELEGBA_HOT_RELOAD` | `true` | Enable/disable hot reload. |
| `ELEGBA_HOT_RELOAD_DEBOUNCE` | `500ms` | Debounce interval for file events. |

### Limitations

- Hot reload does not apply to `server.*` fields (address, TLS, timeouts).
  These require a restart.
- Hot reload does not apply to `caches.*` backends (connections are reused).
  Adding/removing caches requires a restart.
- Hot reload does apply to `upstreams.*` and `endpoints.*`.

---

## 13. Complete Example

This example is the canonical multi-upstream aggregation scenario, also
documented in `DESIGN.md` §16 and available as `examples/user-summary.yaml`.

```yaml
version: "1"

server:
  address: ":8080"
  readTimeout: 5s
  writeTimeout: 15s
  idleTimeout: 120s
  readHeaderTimeout: 5s
  shutdownTimeout: 30s
  maxBodyBytes: 1048576
  admin:
    address: ":9090"
    enabled: true

upstreams:
  user-service:
    transport: http
    baseURL: ${USER_SERVICE_URL}
    timeout: 2s
    maxConcurrent: 100
    retries: 2
    retry:
      initialInterval: 100ms
      maxInterval: 1s
      multiplier: 2.0
      maxElapsedTime: 5s
      jitter: true
    breaker:
      maxRequests: 5
      interval: 60s
      timeout: 30s
      failureThreshold: 5
    rateLimit: 200/s
    auth:
      type: client
      header: Authorization
      forward: Authorization
      scheme: Bearer

  ledger-service:
    transport: http
    baseURL: ${LEDGER_SERVICE_URL}
    timeout: 3s
    maxConcurrent: 100
    retries: 2
    breaker:
      failureThreshold: 5
    rateLimit: 200/s
    auth:
      type: client
      header: Authorization
      forward: Authorization
      scheme: Bearer

  account-service:
    transport: http
    baseURL: ${ACCOUNT_SERVICE_URL}
    timeout: 2s
    maxConcurrent: 100
    retries: 3
    breaker:
      failureThreshold: 3
    rateLimit: 200/s
    auth:
      type: client
      header: Authorization
      forward: Authorization
      scheme: Bearer

caches:
  memory:
    type: in-memory
    maxSize: 10000
    defaultTTL: 60s

endpoints:
  - path: /users/{userId}/summary
    method: GET
    timeout: 8s
    maxConcurrent: 200
    failFast: false
    pipeline:
      - id: user
        type: fetch
        upstream: user-service
        path: /users/{{ .path.userId }}
        method: GET
        cache:
          backend: memory
          key: "user:{{ .path.userId }}:{{ .header.Authorization | sha256 }}"
          ttl: 60s
          staleWhileRevalidate: true

      - id: ledger
        type: fetch
        upstream: ledger-service
        path: /ledger/accounts/{{ .path.userId }}
        method: GET

      - id: account
        type: fetch
        upstream: account-service
        path: /accounts/{{ .path.userId }}/flags
        method: GET
        onError: fallback
        fallback:
          static: '{"has_pnd": false, "has_lien": false}'

      - id: summary
        type: transform
        dependsOn: [user, ledger, account]
        template: |
          {{ dict
              "user_id"            .user.id
              "user_status"        .user.user_status
              "user_onboarded_on"  .user.date_create
              "account_balance"    .ledger.ledger_balance
              "account_has_pnd"    .account.has_pnd
              "account_has_lien"   .account.has_lien
            | toJSON }}
```

### Expected request

```bash
curl -s http://localhost:8080/users/42/summary \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  | jq
```

### Expected response

```json
{
  "user_id": 42,
  "user_status": "ACTIVE",
  "user_onboarded_on": "2024-03-15T10:22:00Z",
  "account_balance": 1580.42,
  "account_has_pnd": false,
  "account_has_lien": true
}
```

---

## 14. Migration Guide

### From v0 (pre-release) to v1

- The `version` field is now required. Add `version: "1"` at the top of your
  config.
- `upstreams.*.retries` is now an integer (count), not a duration. Move
  duration configuration to `retry.maxElapsedTime`.
- `endpoints.*.steps` is now `endpoints.*.pipeline`.
- `cache.ttl` moved from a top-level cache field to per-step config.
- `auth.token` now supports `${VAR}` interpolation. If you used literal
  `${...}` before, escape with `$${...}`.

### From v1.0 to v1.1

- New auth type: `auth.type: client`. No changes required for existing
  configs.
- New template functions: `dict`, `sha256`, `replacePrefix`, `redact`. No
  changes required for existing configs.
- Cache safety rule enforced: if `auth.type: client` is used on a cached
  `fetch` step, the cache key must contain a token hash. Existing configs that
  don't use `auth.type: client` are unaffected.

### Future Schema Versions

When the schema changes in a backwards-incompatible way, a new `version` value
will be introduced (e.g., `"2"`). Elegba will support the previous major
version for at least one release cycle, with deprecation warnings.

---

## Appendix A: Type Reference

| Type | Format | Examples |
|------|--------|----------|
| `string` | UTF-8 | `"hello"`, `'world'` |
| `int` | 64-bit signed | `42`, `-1`, `0` |
| `int64` | 64-bit signed | `1048576` |
| `float` | 64-bit float | `2.0`, `0.5` |
| `bool` | Boolean | `true`, `false` |
| `duration` | Go duration string | `"5s"`, `"100ms"`, `"2m30s"` |
| `[]string` | Array of strings | `["a", "b"]` |
| `map[string]string` | String map | `{key: value}` |
| `URL` | Absolute URL | `"https://api.example.com"` |
| `host:port` | Address | `":8080"`, `"127.0.0.1:9090"` |

---

## Appendix B: Env Var Reference

| Variable | Purpose | Default |
|----------|---------|---------|
| `ELEGBA_CONFIG` | Config file path. | `elegba.yaml` |
| `ELEGBA_LOG_LEVEL` | `debug`, `info`, `warn`, `error`. | `info` |
| `ELEGBA_LOG_FORMAT` | `json`, `text`. | `json` |
| `ELEGBA_ADMIN_ADDR` | Admin server address. | — |
| `ELEGBA_CONFIG_MAX_BYTES` | Max config file size. | `10485760` |
| `ELEGBA_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout. | `30s` |
| `ELEGBA_HOT_RELOAD` | Enable hot reload. | `true` |
| `ELEGBA_HOT_RELOAD_DEBOUNCE` | Debounce interval. | `500ms` |
| `ELEGBA_CACHE_BACKEND` | `ristretto` or `bigcache`. | `ristretto` |
| `ELEGBA_OTEL_ENDPOINT` | OTLP endpoint. | — |
| `ELEGBA_OTEL_INSECURE` | Use insecure OTLP. | `false` |

---

## Appendix C: Error Codes

Elegba returns structured errors in JSON:

```json
{
  "error": {
    "code": "UPSTREAM_TIMEOUT",
    "message": "upstream user-service timed out after 2s",
    "requestId": "01HXYZ...",
    "step": "user"
  }
}
```

| Code | HTTP | Description |
|------|------|-------------|
| `CONFIG_INVALID` | 500 | Config failed to load or validate. |
| `ENDPOINT_NOT_FOUND` | 404 | No endpoint matches the path. |
| `METHOD_NOT_ALLOWED` | 405 | Path matches but method does not. |
| `REQUEST_TOO_LARGE` | 413 | Request body exceeds `maxBodyBytes`. |
| `RATE_LIMITED` | 429 | Endpoint-level concurrency or rate limit exceeded. |
| `CLIENT_AUTH_MISSING` | 401 | Client auth header required by upstream is missing. |
| `UPSTREAM_TIMEOUT` | 504 | Upstream timed out. |
| `UPSTREAM_ERROR` | 502 | Upstream returned 5xx or network error. |
| `UPSTREAM_UNAVAILABLE` | 503 | Circuit breaker is open. |
| `TRANSFORM_ERROR` | 500 | Template execution failed. |
| `CACHE_ERROR` | — | Cache error (logged, request continues). |
| `INTERNAL_ERROR` | 500 | Unexpected error. |

---

*End of configuration reference.*