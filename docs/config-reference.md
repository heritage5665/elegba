# Elegba — Configuration Reference

**Version:** 1.0
**Schema Version:** `"1"`
**Applies to:** Elegba v1.x

This document is the exhaustive reference for Elegba's configuration file.
Every field, its type, default value, and behavior is documented here.

The implementation currently supports strict YAML or JSON parsing,
`${VAR}` and `${VAR:-default}` interpolation, server address/timeouts/body
limits, named HTTP upstreams with bearer/basic/apikey auth, retries, circuit
breakers, rate limiting, endpoint/step concurrency and timeouts, partial
failures, fallbacks, named in-memory and Redis caches, and fetch/transform/cache steps.
Prometheus metrics, an optional pprof/health admin listener, readiness probes,
and optional OTLP HTTP tracing are also supported. Upstream/server TLS, CORS,
and hot reload remain planned and are not accepted by the strict loader.

---

## Table of Contents

1. [File Format](#1-file-format)
2. [Environment Variable Interpolation](#2-environment-variable-interpolation)
3. [Top-Level Structure](#3-top-level-structure)
4. [`version`](#4-version)
5. [`server`](#5-server)
6. [`tracing`](#6-tracing)
7. [`upstreams`](#7-upstreams)
8. [`caches`](#8-caches)
9. [`endpoints`](#9-endpoints)
10. [Step Types](#10-step-types)
11. [Template Functions](#11-template-functions)
12. [Validation Rules](#12-validation-rules)
13. [Hot Reload](#13-hot-reload)
14. [Complete Example](#14-complete-example)
15. [Migration Guide](#15-migration-guide)

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

---

## 3. Top-Level Structure

```yaml
version: "1"

server:
  # see §5

tracing:
  # see §6

upstreams:
  # see §7

caches:
  # see §8

endpoints:
  # see §9
```

The configuration sections are optional at the schema level, but Elegba refuses to
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
  readinessTimeout: 5s
  maxBodyBytes: 1048576
  admin:
    address: ":9090"
    enabled: false

tracing:
  enabled: false
  endpoint: http://localhost:4318
  insecure: true
  serviceName: elegba
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
| `readinessTimeout` | duration | no | `5s` | Overall timeout for upstream readiness probes. |
| `maxBodyBytes` | int64 | no | `1048576` (1 MB) | Max incoming request body size. |
| `admin` | object | no | disabled | Separate metrics, pprof, and health listener (§5.3). |

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

The listener is disabled by default. **Security note**: pprof can expose
process information; bind the admin address to a private interface and protect
it with network policy.

## 6. `tracing`

Optional OpenTelemetry tracing exports spans using the OTLP/HTTP protocol.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | no | `false` | Enable OTLP trace export. |
| `endpoint` | URL | required when enabled | — | Collector base URL, for example `http://localhost:4318`. |
| `insecure` | bool | no | `false` | Use an insecure OTLP connection. HTTP endpoints always use cleartext. |
| `serviceName` | string | no | `elegba` | OpenTelemetry `service.name` resource attribute. |

`ELEGBA_OTEL_ENDPOINT` enables tracing and overrides `tracing.endpoint`;
`ELEGBA_OTEL_INSECURE` overrides the `insecure` value. W3C Trace Context is
extracted from incoming `traceparent` and propagated to upstreams.

---

## 7. `upstreams`

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
      type: bearer
      token: ${USER_TOKEN}
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
| `retries` | int | no | `2` | Number of retry attempts (in addition to the first). |
| `retry` | object | no | — | Retry policy (§7.1). |
| `breaker` | object | no | — | Circuit breaker config (§7.2). |
| `rateLimit` | string | no | — | Rate limit as `"N/duration"` (e.g., `"100/s"`). |
| `auth` | object | no | — | Auth config (§7.3). |
| `tls` | object | no | — | Per-upstream TLS config (§7.4). |
| `headers` | map[string]string | no | `{}` | Static headers added to every request. |
| `proxy` | object | no | — | Proxy config (§7.5). |

### 7.1 `upstreams.*.retry`

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
- `retries` is the maximum number of attempts after the first request. The
  default is `2`; set it to `0` to disable retries.
- Rate-limit tokens and upstream concurrency slots apply to every attempt.

### 7.2 `upstreams.*.breaker`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `maxRequests` | int | no | `5` | Max requests allowed in half-open state. |
| `interval` | duration | no | `60s` | Cyclic period for clearing counts in closed state. |
| `timeout` | duration | no | `30s` | How long to stay in open state before half-open. |
| `failureThreshold` | int | no | `5` | Consecutive failures to trip the breaker. |

### 7.3 `upstreams.*.auth`

Supported `type` values: `bearer`, `basic`, `apikey`, `oauth2`.

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

### 7.4 `upstreams.*.tls`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `caFile` | string | no | — | Path to CA bundle. |
| `certFile` | string | no | — | Path to client cert (mTLS). |
| `keyFile` | string | no | — | Path to client key (mTLS). |
| `insecureSkipVerify` | bool | no | `false` | Skip server cert verification. **Not recommended.** |
| `serverName` | string | no | — | Override SNI server name. |

### 7.5 `upstreams.*.proxy`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | Proxy URL (e.g., `http://proxy:3128`). |

---

## 8. `caches`

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
    username: elegba
    password: ${REDIS_PASSWORD}
    db: 0
    poolSize: 10
    defaultTTL: 300s
    tls:
      caFile: /etc/elegba/redis-ca.pem
```

### Fields (common)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `type` | string | no | `"in-memory"` | Cache type: `"in-memory"` or `"redis"`. |
| `defaultTTL` | duration | no | `60s` | Default TTL when a step doesn't specify one. |

### 8.1 `type: in-memory`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `maxSize` | int | no | `10000` | Max number of entries. |
The in-memory backend uses Ristretto with one cost unit per entry; `maxSize`
sets its maximum cost.

### 8.2 `type: redis`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | yes | — | Redis address (host:port). |
| `username` | string | no | — | Redis ACL username. |
| `password` | string | no | — | Redis password. Supports `${VAR}`. |
| `db` | int | no | `0` | Redis database number. |
| `poolSize` | int | no | `10` | Connection pool size. |
| `maxRetries` | int | no | `3` | Max retries on Redis errors. |
| `dialTimeout` | duration | no | `5s` | Connection dial timeout. |
| `readTimeout` | duration | no | `3s` | Read timeout. |
| `writeTimeout` | duration | no | `3s` | Write timeout. |
| `tls` | object | no | — | Redis TLS settings. |

`tls` supports `caFile`, `certFile`, `keyFile`, `serverName`, and
`insecureSkipVerify`. Client `certFile` and `keyFile` must be set together.
Redis request-time errors are logged and treated as cache misses/write
failures; they do not fail otherwise successful API requests.

---

## 9. `endpoints`

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
          key: "user:{{ .query.userId }}"
          staleWhileRevalidate: true
      - id: orders
        type: fetch
        upstream: order-service
        path: /orders?userId={{ .query.userId }}
      - id: combined
        type: transform
        dependsOn: [user, orders]
        template: |
          {
            "user": {{ .user | toJSON }},
            "orderCount": {{ len .orders }}
          }
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `path` | string | yes | — | URL path. Supports `{param}` segments. |
| `method` | string | yes | — | HTTP method (`GET`, `POST`, `PUT`, `DELETE`, `PATCH`). |
| `timeout` | duration | no | `10s` | Per-request timeout for the whole pipeline. |
| `maxConcurrent` | int | no | `100` | Max concurrent requests for this endpoint. |
| `failFast` | bool | no | `false` | If `true`, abort on first step failure. |
| `cacheInvalidate` | []string | no | `[]` | Templated keys deleted from every configured backend before the pipeline runs. |
| `pipeline` | []Step | yes | — | Ordered list of steps (see §10). |

**Path parameters**: segments like `/users/{id}` are extracted into
`{{ .path.id }}` in templates.

**Query parameters**: available as `{{ .query.paramName }}`.

**Headers**: available as `{{ .header.HeaderName }}`.

**Body**: available as `{{ .body }}` (parsed JSON) or `{{ .bodyRaw }}` (raw
string).

**Request ID**: available as `{{ .requestID }}`.

At `maxConcurrent`, additional requests to an endpoint receive `429` with
`Retry-After: 1`. Upstream concurrency limits wait for an available slot until
the request context is canceled or times out.

---

## 10. Step Types

Each step has a `type` field that determines its behavior. All steps share
these fields:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | string | yes | — | Unique step ID within the pipeline. |
| `type` | string | yes | — | Step type: `fetch`, `transform`, `cache`. |
| `dependsOn` | []string | no | `[]` | Step IDs this step depends on. |
| `onError` | string | no | `"fail"` | Error handling: `fail`, `ignore`, `fallback`. |
| `fallback` | object | no | — | Fallback config (if `onError: fallback`). |

### 10.1 `type: fetch`

Calls an upstream API.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `upstream` | string | yes | — | Name of the upstream to call. |
| `path` | string | yes | — | Path template (supports Go template syntax). |
| `method` | string | no | `"GET"` | HTTP method. |
| `headers` | map[string]string | no | `{}` | Additional headers (templated). |
| `query` | map[string]string | no | `{}` | Additional query params (templated). |
| `body` | string | no | — | Request body (templated). |
| `cache` | object | no | — | Cache config for this step (§10.1.1). |

#### 10.1.1 `fetch.cache`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `backend` | string | yes | — | Cache name from `caches`. |
| `key` | string | yes | — | Cache key template. |
| `ttl` | duration | no | backend default | TTL for this entry. |
| `staleWhileRevalidate` | bool | no | `false` | Keep an additional TTL of stale data, serve it immediately, and coalesce background refreshes. |

### 10.2 `type: transform`

Renders a Go template using previous step outputs.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `template` | string | yes | Go template. See §11 for functions. |

The template receives all prior step outputs as top-level variables keyed by
step ID.

### 10.3 `type: cache`

Explicit cache read, write, or delete (for use outside fetch steps).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | yes | `"get"`, `"set"`, or `"delete"`. |
| `backend` | string | yes | Cache name from `caches`. |
| `key` | string | yes | Cache key template. |
| `value` | string | no (required for `set`) | ID of the dependency step whose result to store. |
| `ttl` | duration | no | TTL for `set`. |

Cache misses return `null`. Cache backend errors are logged and treated as
misses or skipped writes/deletes so the request can continue.

### 10.4 `onError` Behavior

- `fail` (default): step failure aborts the pipeline if `failFast: true`,
  otherwise records the error and continues. Failed dependencies are marked
  complete, so downstream steps run and may themselves report an error if a
  required value is missing.
- `ignore`: step failure is silently ignored; the step's output is `null`.
- `fallback`: step failure triggers the `fallback` config.

When `failFast: false`, successful output is returned with `_errors`, keyed by
step ID. Fatal errors use `{ "error": { "code", "message", "request_id" } }`.
Public error messages are sanitized and do not include upstream response
bodies.

#### `fallback`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `static` | string | no | Static JSON to return. |
| `upstream` | string | no | Alternate upstream name. |
| `cacheKey` | string | no | Alternate cache key. |

Only one of `static`, `upstream`, or `cacheKey` may be set. `static` must be
valid JSON. An `upstream` fallback repeats the fetch step's method, path,
headers, and body against the alternate upstream. A `cacheKey` fallback checks
configured in-memory caches in lexicographic backend-name order and uses the
first matching entry.

---

## 11. Template Functions

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
| `split` | Split string. | `{{ split .csv "," }}` |
| `join` | Join slice. | `{{ join .ids "," }}` |
| `contains` | Substring check. | `{{ contains .s "foo" }}` |

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

## 12. Validation Rules

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
- `auth.type` must be a known type.
- `rateLimit` must match `N/duration` format.
- If `auth.type: oauth2`, `tokenURL` must be a valid URL.

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

## 13. Hot Reload

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

## 14. Complete Example

```yaml
version: "1"

server:
  address: ":8080"
  readTimeout: 5s
  writeTimeout: 10s
  idleTimeout: 120s
  readHeaderTimeout: 5s
  shutdownTimeout: 30s
  maxBodyBytes: 1048576
  cors:
    allowedOrigins: ["https://app.example.com"]
    allowedMethods: ["GET", "POST"]
    allowedHeaders: ["Authorization", "Content-Type"]
    allowCredentials: true
    maxAge: 600s
  admin:
    address: ":9090"
    enabled: true

upstreams:
  user-service:
    transport: http
    baseURL: https://api.example.com
    timeout: 2s
    maxResponseBytes: 10485760
    maxConcurrent: 100
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
      type: bearer
      token: ${USER_TOKEN:?USER_TOKEN is required}
    headers:
      X-Client: elegba

  order-service:
    transport: http
    baseURL: https://orders.example.com
    timeout: 3s
    maxConcurrent: 50
    retries: 3
    auth:
      type: apikey
      key: ${ORDER_API_KEY}
      in: header
      name: X-API-Key

caches:
  memory:
    type: in-memory
    maxSize: 10000
    defaultTTL: 60s
  redis:
    type: redis
    address: ${REDIS_ADDR:-localhost:6379}
    password: ${REDIS_PASSWORD:-}
    db: 0
    defaultTTL: 300s
    poolSize: 10

endpoints:
  - path: /dashboard
    method: GET
    timeout: 10s
    maxConcurrent: 100
    failFast: false
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
          key: "user:{{ .query.userId }}"
          staleWhileRevalidate: true

      - id: orders
        type: fetch
        upstream: order-service
        path: /orders
        method: GET
        query:
          userId: "{{ .query.userId }}"
        onError: fallback
        fallback:
          static: "[]"

      - id: combined
        type: transform
        dependsOn: [user, orders]
        template: |
          {
            "user": {{ .user | toJSON }},
            "orderCount": {{ len .orders }},
            "total": {{ sum .orders "amount" }},
            "generatedAt": "{{ now }}"
          }

  - path: /users/{id}
    method: GET
    pipeline:
      - id: user
        type: fetch
        upstream: user-service
        path: /users/{{ .path.id }}
        cache:
          backend: redis
          ttl: 300s
          key: "user:{{ .path.id }}"

  - path: /users/{id}
    method: DELETE
    pipeline:
      - id: invalidate
        type: cache
        action: set
        backend: redis
        key: "user:{{ .path.id }}"
        value: "null"
        ttl: 1s
      - id: delete
        type: fetch
        upstream: user-service
        path: /users/{{ .path.id }}
        method: DELETE
        dependsOn: [invalidate]
```

---

## 15. Migration Guide

### From v0 (pre-release) to v1

- The `version` field is now required. Add `version: "1"` at the top of your
  config.
- `upstreams.*.retries` is now an integer (count), not a duration. Move
  duration configuration to `retry.maxElapsedTime`.
- `endpoints.*.steps` is now `endpoints.*.pipeline`.
- `cache.ttl` moved from a top-level cache field to per-step config.
- `auth.token` now supports `${VAR}` interpolation. If you used literal
  `${...}` before, escape with `$${...}`.

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
| `UPSTREAM_TIMEOUT` | 504 | Upstream timed out. |
| `UPSTREAM_ERROR` | 502 | Upstream returned 5xx or network error. |
| `UPSTREAM_UNAVAILABLE` | 503 | Circuit breaker is open. |
| `TRANSFORM_ERROR` | 500 | Template execution failed. |
| `CACHE_ERROR` | — | Cache error (logged, request continues). |
| `INTERNAL_ERROR` | 500 | Unexpected error. |

---

*End of configuration reference.*