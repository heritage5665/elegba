# Elegba — Design Specification

> **Many paths in. One path out.**
>
> A config-driven API aggregator and Backend-for-Frontend (BFF) written in Go.

**Version:** 1.1
**Status:** Draft
**Language:** Go 1.22+
**License:** MIT

---

## About the Name

**Elegba** (Yoruba: **Ẹlẹ́gba**, a path of **Èṣù**) is named after the Òrìṣà
who stands at the crossroads, carries messages between realms, and opens the
way for communication to flow. In Ìṣẹ̀ṣe and related African diasporic
traditions, Èṣù is the guardian of beginnings, the messenger, and the one who
makes exchange possible between separate worlds.

This project borrows that metaphor deliberately: Elegba sits at the crossroads
between clients and backend services, receives their requests, carries them
across to many upstreams, and returns a unified response. It opens paths.

### A Note of Respect

Èṣù Ẹlẹ́gba is a living, venerated figure in Yoruba and African diasporic
spiritual traditions (including Candomblé, Santería, Lucumí, and Ifá). This
project is named in honor of that tradition, not as a claim of spiritual
association, endorsement, or authority.

We explicitly reject the colonial-era mistranslation of Èṣù as a "devil" or
"demon" — a distortion introduced by early European writers that continues to
misrepresent the tradition today. Èṣù is not a trickster in the malicious
sense; he is the principle of communication, choice, and the opening of
possibility.

This software has no religious function. If you practice a tradition in which
Èṣù is venerated and you would prefer we use a different name, we welcome that
conversation — please open an issue or reach out.

### Pronunciation

- **Elegba** — eh-LEH-gbah
- **Ẹlẹ́gba** — eh-LEH-gbah (with Yoruba tonal marks)
- **Èṣù** — eh-SHOO

---

## 1. Overview

Elegba is a config-driven API aggregator and Backend-for-Frontend (BFF) written
in Go. It reads a YAML or JSON specification that defines upstream APIs,
transport settings, caching rules, and response transformations. Incoming
requests are mapped to a pipeline of steps that execute concurrently, aggregate
data, and return a transformed response.

The program is designed to be lightweight, extensible, and easy to deploy as a
single static binary.

### 1.1 What Elegba Is

- A **declarative BFF**: define endpoints, upstreams, and transformations in
  config; no code required per endpoint.
- A **concurrent aggregator**: fan out to many upstreams in parallel using a
  dependency graph (DAG).
- A **response shaper**: transform and merge upstream responses using Go
  templates.
- A **resilient proxy**: retries, circuit breakers, rate limits, and timeouts
  applied per upstream.
- A **cache-aware layer**: cache any upstream response with configurable TTL
  and invalidation.

### 1.2 What Elegba Is Not

- Not a full API gateway (no auth server, no developer portal, no billing).
- Not a service mesh (no sidecar, no mTLS orchestration).
- Not an orchestration engine for long-running workflows (no durable state).
- Not a UI-driven tool (config is file-based).

---

## 2. Goals

- **Config as the single source of truth** — Upstreams, endpoints, caching, and
  transformations are declared entirely in YAML/JSON.
- **Concurrent aggregation** — Independent API calls run in parallel using a
  dependency graph (DAG).
- **Pluggable components** — Transport, cache, and transformation layers are
  interfaces with multiple implementations.
- **Graceful failure** — Support partial failures, timeouts, retries, and
  circuit breakers.
- **Observability** — Structured logging, Prometheus metrics, and OpenTelemetry
  tracing out of the box.
- **Open-source friendly** — Simple CLI, clear documentation, minimal
  dependencies, and a versioned config schema.
- **Performance** — Zero-allocation hot paths where feasible; pre-parsed
  templates; connection pooling; bounded concurrency.
- **Reliability** — No goroutine leaks, no data races, deterministic output.

---

## 3. Non-Goals

- Full API gateway features beyond what is needed for aggregation.
- Complex business logic or domain modeling inside the aggregator.
- A graphical UI for configuration.
- Persistent state or long-running workflow orchestration.
- Multi-tenant isolation (single-tenant per process).

---

## 4. Architecture

### 4.1 High-Level Diagram

```
┌─────────────┐     ┌─────────────────┐     ┌──────────────────────┐
│ Config File │────▶│ Config Loader   │────▶│ Validated Config     │
│ (YAML/JSON) │     │ + Validator     │     │ (in-memory structs)  │
└─────────────┘     └─────────────────┘     └──────────┬───────────┘
                                                       │
                                                       ▼
┌─────────────┐     ┌─────────────────┐     ┌──────────────────────┐
│ HTTP Request│────▶│ Router          │────▶│ Pipeline Executor    │
│             │     │ (match endpoint)│     │ (DAG of steps)       │
└─────────────┘     └─────────────────┘     └──────────┬───────────┘
                                                       │
                       ┌───────────────────────────────┼───────────────────────────────┐
                       │                               │                               │
                       ▼                               ▼                               ▼
                ┌─────────────┐                 ┌─────────────┐                 ┌─────────────┐
                │ Fetch Step  │                 │ Transform   │                 │ Cache Step  │
                │ (Transport) │                 │ Step        │                 │ (Cache)     │
                └─────────────┘                 └─────────────┘                 └─────────────┘
```

### 4.2 Layered Architecture (Hexagonal)

```
┌─────────────────────────────────────────────────────────────────┐
│                        Domain (pure)                            │
│  engine/    pipeline/    step/                                  │
│  (no I/O, no framework imports, testable in isolation)          │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ ports (interfaces)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Application                                │
│  config/    (orchestration, wiring, lifecycle)                  │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ adapters
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Infrastructure                               │
│  transport/   cache/   transform/   observability/              │
│  (HTTP, Redis, templates, Prometheus, OTel)                     │
└─────────────────────────────────────────────────────────────────┘
```

### 4.3 Core Components

- **Config Loader** — Reads YAML/JSON, expands env vars, validates against a
  schema, watches for changes.
- **Router** — Maps incoming HTTP requests to an endpoint definition based on
  path and method.
- **Pipeline Executor** — Builds a DAG from step dependencies, executes steps
  concurrently, and collects results.
- **Steps** — Individual units of work: `Fetch`, `Transform`, `Cache`, etc.
- **Transport** — Abstraction for calling upstream APIs (HTTP, gRPC, etc.).
- **Cache** — Abstraction for caching layer (in-memory, Redis, etc.).
- **Observability** — Logging, metrics, and tracing.

---

## 5. Design Patterns

The following patterns are used deliberately, not gratuitously.

| Pattern | Where | Why |
|---------|-------|-----|
| **Strategy** | `Transport`, `Cache`, `Step` | Runtime selection from config. |
| **Registry** | Step types, transports, cache backends | Self-registration via factory map. |
| **Builder** | `Pipeline`, `Upstream` construction | Complex config → validated objects. |
| **Chain of Responsibility** | Request middleware, resilience chain | Composable cross-cutting concerns. |
| **DAG Executor** | Pipeline execution | Concurrent, dependency-aware. |
| **Functional Options** | `NewServer`, `NewClient` | Extensible constructors without boilerplate. |
| **Dependency Injection** | Constructor injection | No globals, no hidden state. |
| **Interface Segregation** | All interfaces | Small (1–3 methods), consumer-defined. |
| **Context Propagation** | Every I/O call | Cancellation, deadlines, tracing. |
| **Result / Error types** | Error handling | Sentinel + wrapped errors, `errors.Is/As`. |

### Anti-Patterns to Avoid

- No `interface{}` — use `any` only when necessary, prefer generics.
- No goroutine leaks — always ensure exit on context cancel.
- No unbounded channels or slices — use semaphores and size limits.
- No `panic` in library code — only in `main` for fatal startup errors.
- No global mutable state.
- No silent error swallowing (`_ = doSomething()` is banned).
- No `time.Sleep` for synchronization — use channels/context.
- No `log.Fatal` outside `main`.
- No reflection in hot paths.
- No dependency added without justification.

---

## 6. Configuration Schema

Configuration is provided in YAML (primary) or JSON. The schema is versioned
and validated at startup. Environment variable interpolation is supported using
`${VAR}` and `${VAR:-default}` syntax. All Elegba-specific environment
variables use the `ELEGBA_` prefix.

### 6.1 Top-Level Sections

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
  admin:
    address: ":9090"
    enabled: false

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
      type: client
      header: Authorization
      forward: Authorization
      scheme: Bearer
    tls:
      caFile: /etc/elegba/ca.pem
      certFile: /etc/elegba/cert.pem
      keyFile: /etc/elegba/key.pem
      insecureSkipVerify: false

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
          key: "user:{{ .query.userId }}:{{ .header.Authorization | sha256 }}"
          staleWhileRevalidate: true
      - id: orders
        type: fetch
        upstream: order-service
        path: /orders?userId={{ .query.userId }}
        dependsOn: []
      - id: combined
        type: transform
        dependsOn: [user, orders]
        template: |
          {{ dict
              "user" .user
              "orderCount" (len .orders)
            | toJSON }}
```

### 6.2 Validation Rules

- `version` must be a known schema version (currently `"1"`).
- `server.address` must be a valid host:port.
- Durations must parse with `time.ParseDuration`.
- `upstreams.*.baseURL` must be a valid absolute URL.
- `endpoints.*.pipeline.*.dependsOn` must reference existing step IDs in the
  same pipeline (no forward references, no cycles).
- Templates must parse at load time.
- All `${VAR}` references must resolve at load time unless a default is given.
- Unknown fields are rejected (`yaml.KnownFields(true)`).
- If `auth.type: client` and the upstream is used by a cached `fetch` step,
  the cache key must contain a token hash (see §14.9).

### 6.3 JSON Schema

A JSON Schema is published at `schema/elegba.schema.json` for editor
autocompletion and external validation.

---

## 7. Engine Details

### 7.1 Router

- Matches `path` and `method` from the request to an endpoint definition.
- Supports path parameters (e.g., `/users/{id}`) via `chi`.
- Injects query params, headers, and body into the pipeline context.
- Returns 404 with a structured error if no endpoint matches.
- Returns 405 if path matches but method does not.

### 7.2 Pipeline Executor

- Each endpoint has a list of steps. Steps may declare `dependsOn`.
- Executor builds a DAG, topologically sorts it, runs independent steps
  concurrently.
- Results stored in a thread-safe map keyed by step ID.
- Uses `context.Context` for cancellation and timeouts.
- Per-endpoint `failFast` flag:
  - `true`: any step failure aborts the pipeline.
  - `false`: collect errors, return partial results.

### 7.3 Step Interface

```go
type Step interface {
    ID() string
    DependsOn() []string
    Execute(ctx context.Context, input Input) (Output, error)
}
```

Common step types:

| Type          | Description                                      |
|---------------|--------------------------------------------------|
| `fetch`       | Calls an upstream API using the transport layer. |
| `transform`   | Renders a Go template using previous outputs.    |
| `cache`       | Gets or sets a cache entry.                      |
| `conditional` | If/else branching (future).                      |
| `loop`        | Iterates over a collection (future).             |

### 7.4 Step Registry

```go
type Factory func(cfg StepConfig) (Step, error)

var registry = map[string]Factory{}

func Register(name string, f Factory) { registry[name] = f }

func Build(cfg StepConfig) (Step, error) {
    f, ok := registry[cfg.Type]
    if !ok {
        return nil, fmt.Errorf("unknown step type %q", cfg.Type)
    }
    return f(cfg)
}
```

---

## 8. Concurrency Model

This is the heart of Elegba.

### 8.1 Execution

- Use `golang.org/x/sync/errgroup` with `errgroup.WithContext` for pipeline
  execution.
- Use `golang.org/x/sync/semaphore` for per-upstream concurrency limits.
- Use `golang.org/x/time/rate` for per-upstream rate limiting.
- Every step execution must be wrapped in `context.WithTimeout`.

### 8.2 Partial Failure

- Support **partial failure** via `failFast: false`:
  - Collect errors from failed steps into a `map[string]error` keyed by step ID.
  - Continue executing steps whose dependencies succeeded.
  - Return the aggregated response with a `_errors` field if any step failed.

### 8.3 Safety

- Ensure **no goroutine leaks**: use `defer cancel()`, wait on all goroutines
  before returning, test with `goleak` in CI.
- Store step results in a concurrent-safe structure:
  - Prefer a `sync.Map` for read-heavy access, or
  - A `map[string]any` guarded by `sync.RWMutex` with pre-sized capacity.
- **Determinism**: the final response shape must not depend on step completion
  order. Transform step reads from the results map, not from a channel.
- **Cancellation**: if `failFast: true` and one step fails, cancel the errgroup
  context immediately and let other steps observe cancellation.
- **Backpressure**: bound concurrency at the endpoint level with a configurable
  `maxConcurrent` (default: 100). Reject with 429 if exceeded.

### 8.4 DAG Construction

- Validate `dependsOn` references at config load time.
- Detect cycles via DFS; reject with a clear error.
- Topological sort via Kahn's algorithm.
- Execute steps in waves: all steps with no pending dependencies run in
  parallel; when a step completes, unblock its dependents.

---

## 9. Transformation

The `transform` step takes previous step outputs as a map and produces a new
JSON object. Go's `text/template` is used with custom functions.

### 9.1 Built-in Functions

| Function | Description |
|----------|-------------|
| `toJSON` | Marshal a value to JSON. |
| `fromJSON` | Unmarshal a JSON string. |
| `jsonPath` | Extract via JSONPath expression. |
| `len` | Length of a slice, map, or string. |
| `index` | Index into a slice or map. |
| `default` | Return default if value is nil/empty. |
| `add`, `sub`, `mul`, `div` | Arithmetic. |
| `upper`, `lower`, `trim` | String manipulation. |
| `now` | Current time (RFC3339). |
| `uuid` | Generate a UUIDv7. |
| `dict` | Build a map from key-value pairs. |
| `sha256` | Hex-encoded SHA-256 of a string. |
| `replacePrefix` | Remove a prefix from a string. |
| `redact` | Redact a value for safe logging. |

### 9.2 Example

```gotemplate
{{ dict
    "user" .user
    "orderCount" (len .orders)
  | toJSON }}
```

### 9.3 Performance

- Pre-parse templates at config load time.
- Use `sync.Pool` for template execution contexts.
- Avoid reflection in the hot path.

### 9.4 Alternatives Considered

- **JMESPath** — simpler for extraction, weaker for shaping.
- **JSONata** — powerful, but adds a heavy dependency.
- **Go templates** — chosen for flexibility, Go-native, no runtime deps.

---

## 10. Caching

### 10.1 Cache Interface

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Close() error
}
```

### 10.2 Backends

| Backend   | Library / Notes           | Use Case |
|-----------|---------------------------|----------|
| In-memory | `ristretto` or `bigcache` | Single instance, low latency |
| Redis     | `go-redis`                | Shared across instances |
| Memcached | Optional                  | Legacy environments |

### 10.3 Configuration

- `key`: template string (e.g., `"user:{{ .query.userId }}"`).
- `ttl`: duration.
- `staleWhileRevalidate`: serve stale while refreshing in background.
- Invalidation: write endpoints may declare `cacheInvalidate` key templates.

### 10.4 Semantics

- Cache misses return `ErrCacheMiss` (sentinel).
- Cache errors are logged but never fail the request (fail-open).
- TTL is enforced by the backend; Elegba does not track expiry itself.

---

## 11. Transport Layer

### 11.1 Transport Interface

```go
type Transport interface {
    Do(ctx context.Context, req Request) (Response, error)
    Close() error
}
```

### 11.2 Resilience Chain

Every upstream call is wrapped in a resilience chain:

```
[Rate Limiter] → [Circuit Breaker] → [Retry w/ Backoff] → [Timeout] → HTTP
```

### 11.3 HTTP Implementation

- `net/http` client with configurable timeout, TLS, proxy.
- Retries with exponential backoff (`cenkalti/backoff/v4`) + jitter.
- Circuit breaker (`sony/gobreaker`).
- Rate limiting (`golang.org/x/time/rate`).
- Auth: Bearer, Basic, API Key, OAuth2 client credentials, Client (forward).
- Connection pooling: `MaxIdleConns`, `MaxIdleConnsPerHost`, `IdleConnTimeout`.

### 11.4 Retry Policy

- Retry only idempotent methods (GET, HEAD, OPTIONS) by default.
- Retry on: network errors, 5xx, 429 (respect `Retry-After`).
- Do NOT retry on 4xx (except 429).

### 11.5 Future Transports

- gRPC
- GraphQL
- WebSocket

---

## 12. Observability

### 12.1 Logging

- `log/slog` with a JSON handler.
- Structured fields: `request_id`, `endpoint`, `step_id`, `upstream`,
  `duration_ms`, `status`.
- Log levels:
  - `debug` — step I/O.
  - `info` — request lifecycle.
  - `warn` — retries, cache misses on critical paths.
  - `error` — failures.

### 12.2 Metrics (Prometheus)

| Metric | Type | Labels |
|--------|------|--------|
| `elegba_requests_total` | Counter | `endpoint`, `method`, `status` |
| `elegba_request_duration_seconds` | Histogram | `endpoint` |
| `elegba_step_duration_seconds` | Histogram | `endpoint`, `step`, `upstream` |
| `elegba_upstream_errors_total` | Counter | `upstream`, `reason` |
| `elegba_cache_hits_total` | Counter | `backend` |
| `elegba_cache_misses_total` | Counter | `backend` |
| `elegba_circuit_breaker_state` | Gauge | `upstream` (0=closed, 1=half-open, 2=open) |
| `elegba_inflight_requests` | Gauge | `endpoint` |
| `elegba_client_auth_missing_total` | Counter | `upstream` |

### 12.3 Tracing (OpenTelemetry)

- One span per request, one child span per step, one child span per upstream
  call.
- Propagate `traceparent` headers to upstreams.
- Export via OTLP (configurable endpoint).

### 12.4 Request ID

- Generate a ULID or UUIDv7 per request.
- Inject into context.
- Propagate as `X-Request-ID`.

### 12.5 Health Checks

- `/healthz` — process alive (always 200 if process is running).
- `/readyz` — all upstreams reachable within `readinessTimeout`.
- `/metrics` — Prometheus scrape endpoint (on admin port).

---

## 13. Resilience

### 13.1 Timeouts

Two levels:
- Per-upstream `timeout` (default 2s).
- Per-request `timeout` at the endpoint level (default 10s).

### 13.2 Retries

Exponential backoff with jitter. Configurable per upstream. Idempotent methods
only by default.

### 13.3 Circuit Breaker

Three states: closed, open, half-open. Configurable thresholds. Per upstream.

### 13.4 Bulkhead

Per-upstream `maxConcurrent` via semaphore. Requests that exceed the limit
either queue (up to a bound) or fail fast with 503.

### 13.5 Fallback

Optional per-step `fallback`:
- Static JSON response.
- Alternate upstream.
- Alternate cache key.

### 13.6 Graceful Shutdown

On SIGTERM/SIGINT:
1. Stop accepting new requests.
2. Drain in-flight requests up to `shutdownTimeout` (default 30s).
3. Close upstream clients and cache connections.
4. Exit.

---

## 14. Security

### 14.1 TLS

- Server-side TLS configurable.
- Per-upstream TLS configurable (CA bundle, client cert,
  `InsecureSkipVerify` off by default).

### 14.2 Secrets

- Never log tokens, passwords, or API keys.
- Redact in structured logs and error messages.
- Support env var interpolation for secrets.

### 14.3 Headers

- Strip hop-by-hop headers (`Connection`, `Keep-Alive`, etc.) when proxying.

### 14.4 SSRF Protection

- Validate upstream URLs against a configurable allowlist if enabled.

### 14.5 Size Limits

- `http.MaxBytesReader` on incoming bodies (default 1MB).
- `MaxResponseBytes` per upstream (default 10MB).

### 14.6 CORS

- Configurable allowlist.
- Never `*` with credentials.

### 14.7 Slowloris Protection

- `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout` on the
  server.

### 14.8 Dependency Scanning

- `govulncheck` in CI.

### 14.9 Client-Provided Auth

When an upstream uses `auth.type: client`:

- The client's token is extracted from the incoming request and forwarded to
  the upstream.
- Tokens MUST NOT appear in logs. Redact `Authorization` and any custom auth
  headers.
- Cache keys MUST include a hash of the token to prevent cross-user data
  leakage. Elegba enforces this at config load time.
- TLS MUST be enforced between Elegba and the upstream.
- Per-upstream auth config MUST be deliberate: do not forward a token to an
  upstream that should not see it.
- If the client header is missing, return `401 CLIENT_AUTH_MISSING`.

---

## 15. Project Layout

```
elegba/
├── cmd/
│   └── elegba/
│       └── main.go                # wiring only, no business logic
├── internal/
│   ├── config/
│   │   ├── config.go              # structs
│   │   ├── loader.go              # load + env interpolation
│   │   ├── validate.go            # validation
│   │   ├── watch.go               # hot reload
│   │   └── defaults.go
│   ├── engine/
│   │   ├── engine.go              # top-level orchestrator
│   │   ├── router.go              # request → endpoint
│   │   └── executor.go            # DAG execution
│   ├── pipeline/
│   │   ├── dag.go                 # topological sort
│   │   ├── step.go                # Step interface
│   │   └── result.go              # concurrent result store
│   ├── step/
│   │   ├── registry.go
│   │   ├── fetch.go
│   │   ├── transform.go
│   │   └── cache.go
│   ├── transport/
│   │   ├── transport.go           # interface
│   │   ├── http.go                # HTTP impl
│   │   ├── auth.go                # auth strategies
│   │   ├── retry.go
│   │   ├── breaker.go
│   │   └── ratelimit.go
│   ├── cache/
│   │   ├── cache.go               # interface
│   │   ├── memory.go
│   │   └── redis.go
│   ├── transform/
│   │   ├── template.go
│   │   └── funcs.go               # template funcs
│   └── observability/
│       ├── logging.go
│       ├── metrics.go
│       └── tracing.go
├── pkg/
│   └── elegba/                    # public embedding API
│       └── elegba.go
├── examples/
│   ├── elegba.yaml                # minimal config
│   ├── user-summary.yaml          # §16 worked example
│   ├── dashboard.yaml             # BFF dashboard pattern
│   ├── mobile-bff.yaml            # mobile BFF pattern
│   └── mock/
│       └── main.go                # mock upstreams for testing
├── docs/
│   ├── benchmarks.md
│   ├── config-reference.md
│   ├── architecture.md
│   └── roadmap.md
├── schema/
│   └── elegba.schema.json
├── testdata/
│   └── ...
├── DESIGN.md
├── README.md
├── CONTRIBUTING.md
├── CODE_OF_CONDUCT.md
├── LICENSE
├── Makefile
├── Dockerfile
├── docker-compose.yml
├── .golangci.yml
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
└── go.mod
```

---

## 16. Worked Example — Multi-Upstream Aggregation

This section walks through a complete, concrete example that exercises the
core features of Elegba: parallel fan-out, client-provided auth, field
renaming, partial failure handling, safe caching, and observability.

It is the canonical "hello world" for Elegba and should be used as the
reference example in `examples/` and in the README quick start.

### 16.1 Scenario

**Endpoint:** `GET /users/{userId}/summary`

**Goal:** Return a single JSON response that combines data from three
upstream services:

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

**Upstreams:**

| Upstream | Endpoint | Returns |
|----------|----------|---------|
| `user-service` | `GET /users/{id}` | `{ "id", "user_status", "date_create" }` |
| `ledger-service` | `GET /ledger/accounts/{id}` | `{ "account_number", "ledger_balance" }` |
| `account-service` | `GET /accounts/{id}/flags` | `{ "has_pnd", "has_lien" }` |

**Authentication:** The client sends `Authorization: Bearer <token>`. Elegba
forwards the token to all three upstreams.

**Resilience:** Each upstream has its own timeout, retries, breaker, and rate
limit. Partial failures are allowed and surfaced via fallbacks or an `_errors`
map.

**Caching:** User data cached for 60s, keyed by user ID and a hash of the
client token so users never see each other's cached data.

### 16.2 Field Mapping

| Output field | Source | Source field |
|--------------|--------|--------------|
| `user_id` | user-service | `id` |
| `user_status` | user-service | `user_status` |
| `user_onboarded_on` | user-service | `date_create` |
| `account_balance` | ledger-service | `ledger_balance` |
| `account_has_pnd` | account-service | `has_pnd` |
| `account_has_lien` | account-service | `has_lien` |

### 16.3 Configuration

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

### 16.4 Execution Trace

For `GET /users/42/summary` with `Authorization: Bearer eyJ...`:

```
t=0ms   Middleware
        ├── generate request ID
        ├── attach to context
        ├── start metrics timer
        └── start trace span

t=1ms   Router
        └── match /users/{userId}/summary, extract userId=42

t=1ms   Executor builds DAG
        Wave 1: user, ledger, account (no dependencies)
        Wave 2: summary (depends on all three)

t=2ms   Wave 1 (concurrent)
        ├── fetch user    → cache miss → GET user-service
        ├── fetch ledger  → GET ledger-service
        └── fetch account → GET account-service
              (all three carry the client's Authorization header)

t=45ms  Wave 1 complete, results stored

t=46ms  Wave 2
        └── transform summary → dict + toJSON

t=47ms  Response
        ├── 200 OK
        ├── Content-Type: application/json
        ├── X-Request-ID: 01HXYZ...
        └── logs, metrics, trace emitted
```

### 16.5 Partial Failure Behavior

If `account-service` is unavailable, the fallback returns
`{"has_pnd": false, "has_lien": false}`, and the response still contains all
six fields. The client gets usable data instead of a 500.

If the fallback is removed and `failFast: false`, the response includes an
`_errors` map:

```json
{
  "user_id": 42,
  "user_status": "ACTIVE",
  "user_onboarded_on": "2024-03-15T10:22:00Z",
  "account_balance": 1580.42,
  "account_has_pnd": null,
  "account_has_lien": null,
  "_errors": {
    "account": "upstream account-service unavailable: circuit breaker open"
  }
}
```

Both behaviors are valid; the choice depends on whether the client prefers
silence or transparency.

### 16.6 Cache Safety

The cache key is:

```
user:{userId}:{sha256(Authorization)}
```

Including the token hash ensures that two different users requesting the same
userId never share a cache entry. Omitting it would leak data across users —
a critical correctness and security bug. Elegba enforces this at config load
time.

### 16.7 Client-Provided Auth

All three upstreams declare:

```yaml
auth:
  type: client
  header: Authorization
  forward: Authorization
  scheme: Bearer
```

This tells Elegba to extract the client's `Authorization` header, strip any
existing scheme, re-add `Bearer`, and send it to the upstream. If the client
header is missing, Elegba returns `401 CLIENT_AUTH_MISSING`.

### 16.8 Observability Signals

| Signal | What to observe |
|--------|-----------------|
| `elegba_requests_total{endpoint="/users/{userId}/summary",status="200"}` | Request volume |
| `elegba_request_duration_seconds{endpoint="/users/{userId}/summary"}` | Latency distribution |
| `elegba_step_duration_seconds{step="user"}` | Per-step latency |
| `elegba_cache_hits_total{backend="memory"}` | Cache effectiveness |
| `elegba_upstream_errors_total{upstream="account-service"}` | Upstream health |
| `elegba_circuit_breaker_state{upstream="account-service"}` | Breaker state |
| Logs with `request_id`, `endpoint`, `step_id`, `duration_ms` | Debugging |
| Trace spans `request → step → upstream` | Distributed tracing |

### 16.9 What This Example Demonstrates

- **Parallel fan-out** to three upstreams.
- **Client-provided auth** forwarded per upstream.
- **Field renaming and reshaping** via `dict` + `toJSON`.
- **Partial failure handling** via `failFast: false` and `onError: fallback`.
- **Safe caching** keyed by user and token hash.
- **Per-upstream resilience** with independent timeouts, retries, breakers.
- **Zero handler code** — the entire feature is one YAML file.

### 16.10 Required Template Functions

This example relies on two template functions that MUST be included in
`internal/transform/funcs.go`:

| Function | Purpose | Signature |
|----------|---------|-----------|
| `dict` | Build a map from key-value pairs. | `dict(k1, v1, k2, v2, ...) map[string]any` |
| `sha256` | Hex-encoded SHA-256 of a string. | `sha256(s string) string` |

Both are trivial to implement and are used throughout the docs and examples.

### 16.11 Client Auth — Config Reference Addition

The `auth` object under `upstreams.*` gains a new type:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `type` | string | yes | — | Now includes `"client"`. |
| `header` | string | no | `"Authorization"` | Incoming header to read the token from. |
| `forward` | string | no | same as `header` | Outgoing header to send. |
| `scheme` | string | no | `"Bearer"` | Scheme prefix to apply. Set to `""` to forward verbatim. |

**Behavior:**

1. Extract `.header.<header>` from the incoming request.
2. Strip the existing scheme prefix if present.
3. Re-add the configured `scheme` prefix.
4. Set the `forward` header on the upstream request.
5. If the client's header is missing → return `401 CLIENT_AUTH_MISSING`.

**Security:**

- Tokens are never logged; they are redacted in structured logs.
- Cache keys MUST include a token hash when `auth.type: client` is used.
- TLS MUST be enforced between Elegba and upstreams.

---

## 17. Performance Requirements

- **Zero-allocation hot paths** where feasible. Use `sync.Pool` for
  `bytes.Buffer`, `http.Request` bodies, and template execution contexts.
- **Pre-parse templates** at config load time, not per request.
- **Reuse HTTP clients** per upstream (one `*http.Client` per upstream,
  configured with its own `Transport`, timeouts, connection pool).
- **Connection pooling**: set `MaxIdleConns`, `MaxIdleConnsPerHost`,
  `IdleConnTimeout` on each transport.
- **Avoid reflection** in request/response mapping.
- **Streaming**: for large upstream responses, stream with `io.Copy` where
  transformation allows.
- **JSON**: use `encoding/json` for correctness; consider `goccy/go-json` only
  if benchmarks justify it.
- **Memory limits**: enforce `MaxResponseBytes` per upstream via
  `http.MaxBytesReader`.
- **Benchmarks**: include `BenchmarkPipeline_2Steps`,
  `BenchmarkPipeline_10StepsParallel`, `BenchmarkTransform_Simple` with
  `-benchmem`, documented in `docs/benchmarks.md`.
- **Profiling**: expose `net/http/pprof` on a separate admin port (configurable,
  disabled by default).

---

## 18. Testing Strategy

- **Coverage**: ≥ 85% on `internal/` packages. Enforce in CI.
- **Table-driven tests** for all pure logic (DAG, config, transform).
- **Integration tests** using `httptest.Server` for upstream mocking.
- **Golden file tests** for transformation outputs (`testdata/*.golden`).
- **Race detector**: all tests run with `-race`.
- **Goroutine leak detection**: `go.uber.org/goleak` in `TestMain`.
- **Fuzz tests**: `FuzzConfigLoader`, `FuzzTemplateRender`.
- **Benchmarks**: for executor, transform, transport.
- **Chaos tests**: slow upstreams, 500s, timeouts, connection resets.
- **Contract tests**: verify transformed responses match the config spec.

---

## 19. Build & Deployment

- **Makefile** targets: `build`, `test`, `lint`, `bench`, `cover`, `docker`,
  `run`, `clean`, `tidy`, `generate`.
- **Dockerfile**: multi-stage, distroless or scratch base, non-root user,
  final image < 20MB.
- **Cross-compilation**: `GOOS`/`GOARCH` matrix in release workflow.
- **CI** (GitHub Actions):
  - Lint (`golangci-lint`)
  - Test (`go test -race -cover`)
  - `govulncheck`
  - Build all platforms
  - Publish Docker image on tag
  - Generate SBOM (Syft)
- **Releases**: goreleaser with checksums and signed artifacts.

---

## 20. Development Roadmap

### MVP

- YAML config parsing + validation.
- HTTP transport with basic auth.
- Concurrent fetch steps (DAG).
- Go template transformation.
- In-memory cache.
- Structured logging.

### v1

- Redis cache.
- Retries, circuit breaker, rate limiting.
- Hot reload of config.
- Prometheus metrics + OpenTelemetry tracing.
- Partial failure handling.

### v2

- gRPC transport.
- Conditional / loop steps.
- Plugin system for custom steps (Go plugins or WASM).
- Admin UI for config inspection.

---

## 21. Naming Conventions

| Place              | Value                        |
|--------------------|------------------------------|
| GitHub repo        | `elegba`                     |
| Go module          | `github.com/<you>/elegba`    |
| Binary             | `elegba`                     |
| Docker image       | `elegba:latest`        |
| Helm chart         | `elegba`                     |
| Config file        | `elegba.yaml`                |
| Env var prefix     | `ELEGBA_`                    |
| CLI root command   | `elegba`                     |

---

## 22. Definition of Done

A feature is done when:

- [ ] Code is idiomatic, formatted, and passes all linters.
- [ ] Unit tests exist with ≥ 85% coverage on the package.
- [ ] Integration tests cover the happy path and at least 2 failure modes.
- [ ] Godoc comments exist on all exported symbols.
- [ ] Metrics, logs, and traces are emitted for observable behavior.
- [ ] No goroutine leaks (verified by `goleak`).
- [ ] No data races (verified by `-race`).
- [ ] Benchmarks exist for hot paths.
- [ ] Documentation is updated (README, config reference, architecture).
- [ ] A working example config demonstrates the feature.
- [ ] The worked example from §16 passes end to end via `docker-compose`.

---

## 23. References

- Go concurrency: `goroutines`, `channels`, `context`, `errgroup`.
- Template engine: `text/template`.
- Validation: `go-playground/validator`.
- Caching: `ristretto`, `bigcache`, `go-redis`.
- Resilience: `cenkalti/backoff`, `sony/gobreaker`.
- Observability: `log/slog`, `prometheus/client_golang`, OpenTelemetry.
- Router: `chi`, `httprouter`.
- Leak detection: `go.uber.org/goleak`.
- Vulnerability scanning: `govulncheck`.
- SBOM: `syft`.
- Releases: `goreleaser`.