# Elegba — Architecture

**Version:** 1.0
**Applies to:** Elegba v1.x

This document describes the internal architecture of Elegba: how a request flows
through the system, how the DAG executor works, how components interact, and
the reasoning behind key design decisions.

For the high-level design and goals, see [DESIGN.md](../DESIGN.md).
For configuration fields, see [config-reference.md](./config-reference.md).

---

## Table of Contents

1. [Principles](#1-principles)
2. [System Context](#2-system-context)
3. [Layered Architecture](#3-layered-architecture)
4. [Component Overview](#4-component-overview)
5. [Request Lifecycle](#5-request-lifecycle)
6. [Pipeline Execution Model](#6-pipeline-execution-model)
7. [DAG Executor](#7-dag-executor)
8. [Concurrency Model](#8-concurrency-model)
9. [Resilience Chain](#9-resilience-chain)
10. [Caching Strategy](#10-caching-strategy)
11. [Transformation Engine](#11-transformation-engine)
12. [Configuration Lifecycle](#12-configuration-lifecycle)
13. [Hot Reload](#13-hot-reload)
14. [Observability Pipeline](#14-observability-pipeline)
15. [Error Handling](#15-error-handling)
16. [Graceful Shutdown](#16-graceful-shutdown)
17. [Design Decisions & Trade-offs](#17-design-decisions--trade-offs)
18. [Extension Points](#18-extension-points)
19. [Diagrams](#19-diagrams)

---

## 1. Principles

Elegba is built on these non-negotiable principles:

1. **Config is the single source of truth.** No behavior is hardcoded; every
   endpoint, upstream, cache, and transformation is declared in config.
2. **Domain logic is pure.** The engine, pipeline, and step packages have no
   I/O and no framework imports. They can be tested in isolation.
3. **Ports and adapters.** Interfaces are defined by consumers (the domain),
   implemented by adapters (transport, cache, observability).
4. **Concurrency by default.** Independent steps run in parallel; the DAG
   executor is the heart of the system.
5. **Fail gracefully, never silently.** Errors are collected, categorized, and
   either propagated or recorded — never swallowed.
6. **Observable from day one.** Every request, step, and upstream call emits
   logs, metrics, and traces.
7. **No goroutine leaks.** Every goroutine has a clear lifecycle tied to a
   context.
8. **Deterministic output.** The response shape does not depend on step
   completion order.

---

## 2. System Context

Elegba sits between clients (browsers, mobile apps, other services) and backend
upstream APIs. It aggregates, transforms, and caches their responses.

```
┌──────────────┐        ┌─────────────────┐        ┌─────────────────────┐
│   Clients    │        │     Elegba      │        │  Upstream APIs      │
│              │        │                 │        │                     │
│  - Browser   │◀──────▶│  HTTP Server    │◀──────▶│  - user-service     │
│  - Mobile    │  HTTP  │  Router         │  HTTP  │  - order-service    │
│  - Services  │        │  DAG Executor   │        │  - payment-service  │
│              │        │  Cache Layer    │        │  - inventory        │
└──────────────┘        └────────┬────────┘        └─────────────────────┘
                                 │
                                 │
                        ┌────────▼────────┐
                        │  Observability  │
                        │                 │
                        │  - Prometheus   │
                        │  - OTLP         │
                        │  - stdout logs  │
                        └─────────────────┘
```

**Adjacent systems:**
- **Cache backends**: in-memory, Redis (optional).
- **Secrets**: environment variables, injected by the orchestrator.
- **Config source**: local file (watched for changes).

---

## 3. Layered Architecture

Elegba follows a hexagonal (ports and adapters) architecture with three layers.

```
┌──────────────────────────────────────────────────────────────────┐
│                        Domain (pure)                             │
│                                                                  │
│  engine/     pipeline/     step/                                 │
│                                                                  │
│  - Engine orchestrates request handling                          │
│  - Pipeline builds and executes the DAG                          │
│  - Step defines the unit of work                                 │
│                                                                  │
│  No I/O. No framework imports. Testable in isolation.            │
└──────────────────────────────────────────────────────────────────┘
                              ▲
                              │ ports (interfaces)
                              │
┌──────────────────────────────────────────────────────────────────┐
│                        Application                               │
│                                                                  │
│  config/                                                         │
│                                                                  │
│  - Loads and validates configuration                             │
│  - Watches for changes                                           │
│  - Wires the engine at startup                                   │
│  - Manages lifecycle (start, reload, shutdown)                   │
└──────────────────────────────────────────────────────────────────┘
                              ▲
                              │ adapters
                              │
┌──────────────────────────────────────────────────────────────────┐
│                      Infrastructure                              │
│                                                                  │
│  transport/     cache/     transform/     observability/         │
│                                                                  │
│  - HTTP client, auth strategies, retries, breakers               │
│  - In-memory / Redis caches                                      │
│  - Go templates and custom functions                             │
│  - Logging, metrics, tracing                                     │
└──────────────────────────────────────────────────────────────────┘
```

### Ports (interfaces)

| Port | Defined in | Implemented by |
|------|-----------|----------------|
| `Transport` | `pipeline` | `transport/http` |
| `Cache` | `pipeline` | `cache/memory`, `cache/redis` |
| `Step` | `pipeline` | `step/fetch`, `step/transform`, `step/cache` |
| `Renderer` | `pipeline` | `transform/template` |
| `Logger` | `pipeline` | `observability/logging` |
| `Metrics` | `pipeline` | `observability/metrics` |
| `Tracer` | `pipeline` | `observability/tracing` |

### Dependency direction

Dependencies always point inward:

```
infrastructure → application → domain
```

The domain knows nothing about HTTP, Redis, or Prometheus. It defines
interfaces; the infrastructure implements them.

---

## 4. Component Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                              Engine                                 │
│                                                                     │
│  ┌────────────┐    ┌──────────────┐    ┌───────────────────────┐    │
│  │  Router    │───▶│  Executor    │───▶│  Result Store         │    │
│  │            │    │              │    │                       │    │
│  │  matches   │    │  builds DAG  │    │  map[string]any       │    │
│  │  endpoint  │    │  runs steps  │    │  guarded by RWMutex   │    │
│  └────────────┘    └──────┬───────┘    └───────────┬───────────┘    │
│                            │                        │               │
│                            ▼                        │               │
│                   ┌──────────────────┐              │               │
│                   │   Step Registry  │              │               │
│                   │                  │              │               │
│                   │  fetch           │              │               │
│                   │  transform       │              │               │
│                   │  cache           │              │               │
│                   └────────┬─────────┘              │               │
│                            │                        │               │
│                            ▼                        ▼               │
│                   ┌──────────────────┐    ┌───────────────────┐     │
│                   │  Transport       │    │  Renderer         │     │
│                   │  Cache           │    │  (templates)      │     │
│                   └──────────────────┘    └───────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
```

### Engine

- **Owns** the compiled pipeline definitions, upstream clients, and caches.
- **Exposes** an `http.Handler` interface.
- **Swappable** via `atomic.Pointer[Engine]` for hot reload.

### Router

- Maps `(method, path)` to an endpoint definition.
- Extracts path parameters, query parameters, headers, and body.
- Builds a `RequestContext` passed to the executor.

### Executor

- Builds a DAG from the endpoint's pipeline definition.
- Topologically sorts steps.
- Executes steps in waves, respecting dependencies.
- Collects results and errors in a thread-safe store.

### Result Store

- Holds step outputs keyed by step ID.
- Read-heavy: uses `sync.RWMutex` (or `sync.Map` for very large pipelines).
- Pre-sized to the number of steps to avoid rehashing.

### Step Registry

- Maps step type names to factory functions.
- Populated at startup by `init()` in each step package.
- Allows new step types to be added without modifying the executor.

### Transport

- One instance per upstream.
- Wraps `net/http` with auth, retries, breaker, rate limiter.
- Implements the `Transport` interface defined by the domain.

### Cache

- One instance per named cache in config.
- Implements the `Cache` interface defined by the domain.
- Fail-open: cache errors never fail a request.

### Renderer

- Wraps `text/template`.
- Pre-parses templates at config load time.
- Exposes a `Render(ctx, data) ([]byte, error)` method.

---

## 5. Request Lifecycle

Here's the full path of a request through Elegba.

```
┌────────────┐
│   Client   │
└─────┬──────┘
      │ 1. HTTP request
      ▼
┌──────────────────────────────────────────────────────────────┐
│  2. HTTP Server                                              │
│     - Read body (bounded)                                    │
│     - Apply read/write timeouts                              │
│     - Generate request ID (ULID)                             │
│     - Attach to context                                      │
└─────┬────────────────────────────────────────────────────────┘
      │
      ▼
┌──────────────────────────────────────────────────────────────┐
│  3. Middleware Chain                                         │
│     - Request ID injection                                   │
│     - CORS                                                   │
│     - Recovery (panic → 500)                                 │
│     - Metrics (start timer)                                  │
│     - Tracing (start span)                                   │
│     - Endpoint concurrency limiter (semaphore)               │
└─────┬────────────────────────────────────────────────────────┘
      │
      ▼
┌──────────────────────────────────────────────────────────────┐
│  4. Router                                                   │
│     - Match method + path                                    │
│     - Extract path/query/header/body                         │
│     - 404 if no match, 405 if method mismatch                │
└─────┬────────────────────────────────────────────────────────┘
      │
      ▼
┌──────────────────────────────────────────────────────────────┐
│  5. Executor                                                 │
│     - Build DAG from pipeline definition                     │
│     - Topological sort                                       │
│     - Execute steps in waves (see §7)                        │
│     - Collect results + errors                               │
└─────┬────────────────────────────────────────────────────────┘
      │
      ▼
┌──────────────────────────────────────────────────────────────┐
│  6. Response Assembly                                        │
│     - If final step is transform: use its output             │
│     - Else: assemble from all step outputs                   │
│     - If failFast: false and errors: add _errors field       │
│     - Marshal to JSON                                        │
└─────┬────────────────────────────────────────────────────────┘
      │
      ▼
┌──────────────────────────────────────────────────────────────┐
│  7. Response                                                 │
│     - Set Content-Type: application/json                     │
│     - Set X-Request-ID                                       │
│     - Write status + body                                    │
│     - Emit metrics, close span, log                          │
└──────────────────────────────────────────────────────────────┘
```

### Step 5 in detail

The executor:

1. **Builds** the DAG from the pipeline definition.
2. **Validates** the DAG (no cycles, all `dependsOn` refs exist).
3. **Topologically sorts** the steps.
4. **Runs** steps in waves: all steps whose dependencies are satisfied run
   concurrently.
5. **Waits** for the wave to complete (or for a failure if `failFast: true`).
6. **Repeats** until all steps are done.

---

## 6. Pipeline Execution Model

A pipeline is a list of steps. Each step declares:

- `id` — unique identifier.
- `type` — `fetch`, `transform`, `cache`, etc.
- `dependsOn` — list of step IDs it depends on.

### Example pipeline

```yaml
pipeline:
  - id: user
    type: fetch
    upstream: user-service
    path: /users/{{ .query.userId }}

  - id: orders
    type: fetch
    upstream: order-service
    path: /orders?userId={{ .query.userId }}

  - id: recommendations
    type: fetch
    upstream: rec-service
    path: /recommendations/{{ .query.userId }}
    dependsOn: [user]

  - id: combined
    type: transform
    dependsOn: [user, orders, recommendations]
    template: |
      {
        "user": {{ .user | toJSON }},
        "orders": {{ .orders | toJSON }},
        "recommendations": {{ .recommendations | toJSON }}
      }
```

### Resulting DAG

```
        ┌────────┐     ┌────────┐
        │  user  │     │ orders │
        └───┬────┘     └───┬────┘
            │              │
            ▼              │
    ┌───────────────┐      │
    │recommendations│      │
    └───────┬───────┘      │
            │              │
            └──────┬───────┘
                   ▼
            ┌────────────┐
            │  combined  │
            └────────────┘
```

### Execution waves

| Wave | Steps running concurrently |
|------|----------------------------|
| 1 | `user`, `orders` |
| 2 | `recommendations` (depends on `user`) |
| 3 | `combined` (depends on all) |

### Why waves?

Waves maximize parallelism while respecting dependencies. Steps in the same
wave are guaranteed independent. This makes reasoning about concurrency
simple and testable.

---

## 7. DAG Executor

The executor is the heart of Elegba. It must be correct, concurrent, and
leak-free.

### Algorithm

```
function Execute(pipeline, ctx):
    dag = BuildDAG(pipeline)              # validate, detect cycles
    sorted = TopologicalSort(dag)         # Kahn's algorithm
    store = NewResultStore(len(pipeline))
    errors = NewErrorStore()

    while not dag.IsEmpty():
        wave = dag.ReadySteps()           # steps with no pending deps
        if wave is empty:
            return error "deadlock: cycle detected"

        group, groupCtx = errgroup.WithContext(ctx)

        for step in wave:
            step := step                  # capture loop variable
            group.Go(func() error {
                stepCtx, cancel = context.WithTimeout(groupCtx, step.Timeout)
                defer cancel()

                out, err = step.Execute(stepCtx, buildInput(store, step))

                if err != nil:
                    errors.Record(step.ID, err)
                    if pipeline.FailFast:
                        return err        # cancel group
                    store.Set(step.ID, nil)  # partial failure
                    return nil

                store.Set(step.ID, out)
                return nil
            })

        if err = group.Wait(); err != nil:
            return err

        dag.MarkDone(wave)

    return store
```

### Data structures

```go
type DAG struct {
    steps    map[string]*StepNode
    ready    map[string]bool
    pending  map[string]int          // step ID → unresolved dep count
}

type StepNode struct {
    ID        string
    Step      Step
    DependsOn []string
    Dependents []string
}

type ResultStore struct {
    mu      sync.RWMutex
    results map[string]any
    errors  map[string]error
}
```

### Cycle detection

Kahn's algorithm detects cycles naturally: if the wave is empty but the DAG is
not, there's a cycle. However, Elegba validates the DAG at **config load time**
and rejects cyclic pipelines early, so cycles never reach the executor at
runtime.

### Determinism

The result store is keyed by step ID. The final response is assembled from the
store, not from channel order. This guarantees deterministic output regardless
of step completion order.

### Error semantics

| `failFast` | Behavior |
|-----------|----------|
| `true` | First error cancels the group context; all in-flight steps observe cancellation; pipeline returns error. |
| `false` | Errors are recorded per step; steps whose dependencies succeeded continue; final response includes `_errors` map. |

### Partial failure example

With `failFast: false` and this pipeline:

```yaml
pipeline:
  - id: user
    type: fetch
    upstream: user-service
  - id: orders
    type: fetch
    upstream: order-service
  - id: combined
    type: transform
    dependsOn: [user, orders]
```

If `orders` fails but `user` succeeds:

- `user` → result stored.
- `orders` → error stored, result is `nil`.
- `combined` → executes with `user` present and `orders` as `nil`.
- Response includes:

```json
{
  "user": { ... },
  "orderCount": 0,
  "_errors": {
    "orders": "upstream order-service timed out after 3s"
  }
}
```

---

## 8. Concurrency Model

Elegba uses a **bounded, context-aware** concurrency model.

### Primitives

| Primitive | Package | Purpose |
|-----------|---------|---------|
| `errgroup.Group` | `golang.org/x/sync/errgroup` | Coordinate goroutines, propagate first error. |
| `semaphore.Weighted` | `golang.org/x/sync/semaphore` | Bound per-upstream concurrency. |
| `rate.Limiter` | `golang.org/x/time/rate` | Rate limit per upstream. |
| `context.Context` | stdlib | Cancellation, deadlines, tracing. |
| `sync.RWMutex` | stdlib | Protect the result store. |
| `atomic.Pointer[T]` | stdlib | Hot-swap the engine. |

### Concurrency limits

| Scope | Limit | Config field |
|-------|-------|--------------|
| Endpoint | `maxConcurrent` | `endpoints.*.maxConcurrent` |
| Upstream | `maxConcurrent` | `upstreams.*.maxConcurrent` |
| Step | No explicit limit (bounded by upstream) | — |

### Backpressure

When endpoint concurrency is exceeded:

1. Request waits on the semaphore up to `acquireTimeout`.
2. If timeout: respond 429 Too Many Requests.

When upstream concurrency is exceeded:

1. Request waits on the upstream semaphore.
2. If the endpoint context is cancelled first: step returns context error.

### Goroutine lifecycle

Every goroutine is:

1. **Started** by `errgroup.Go`.
2. **Bounded** by a context derived from the request context.
3. **Waited** on before the executor returns.
4. **Verified** leak-free by `go.uber.org/goleak` in tests.

No goroutine is started without a corresponding wait.

### Cancellation propagation

```
Request context
    └── Endpoint context (timeout)
            └── errgroup context (failFast cancel)
                    └── Step context (per-step timeout)
                            └── Upstream request context
```

Cancellation at any level propagates downward. Steps observe cancellation via
`ctx.Done()`.

---

## 9. Resilience Chain

Every upstream call is wrapped in a resilience chain. The order matters.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Resilience Chain                            │
│                                                                 │
│  ┌──────────────┐                                               │
│  │ Rate Limiter │  Reject if over limit (429)                   │
│  └──────┬───────┘                                               │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │   Bulkhead   │  Bound concurrency (semaphore)                │
│  └──────┬───────┘                                               │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │   Breaker    │  Fail fast if upstream is unhealthy           │
│  └──────┬───────┘                                               │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │    Retry     │  Backoff + jitter                             │
│  └──────┬───────┘                                               │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │   Timeout    │  Per-request deadline                         │
│  └──────┬───────┘                                               │
│         ▼                                                       │
│  ┌──────────────┐                                               │
│  │  HTTP Client │  Connection pool, TLS, proxy                 │
│  └──────────────┘                                               │
└─────────────────────────────────────────────────────────────────┘
```

### Why this order?

1. **Rate limiter first**: reject early, before consuming other resources.
2. **Bulkhead second**: bound concurrency before touching the breaker.
3. **Breaker third**: skip retries entirely if the circuit is open.
4. **Retry fourth**: retries the actual call, not the limiter checks.
5. **Timeout fifth**: bounds each attempt.
6. **HTTP last**: the actual I/O.

### Retry + breaker interaction

- A retry does **not** count as a separate breaker call.
- The breaker counts the **final outcome** of the retry chain (success or
  exhausted retries).
- This prevents a single slow upstream from tripping the breaker on every
  retry.

### Circuit breaker states

```
       failures ≥ threshold
CLOSED ─────────────────────▶ OPEN
  ▲                            │
  │                            │ timeout
  │                            ▼
  │  success              HALF-OPEN
  └────────────────────────────┤
                               │ failure
                               ▼
                             OPEN
```

- **CLOSED**: normal operation. Counts failures.
- **OPEN**: all requests fail fast with `UPSTREAM_UNAVAILABLE`.
- **HALF-OPEN**: allows up to `maxRequests` probes. Success → CLOSED; failure → OPEN.

---

## 10. Caching Strategy

Caching is applied **per step**, not per endpoint. This gives fine-grained
control.

### Read path

```
Step execution
    │
    ▼
┌─────────────────┐
│ Cache lookup    │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
   HIT      MISS
    │         │
    ▼         ▼
Return     Call upstream
cached         │
value          ▼
          Store in cache
               │
               ▼
          Return value
```

### Cache key

Cache keys are **templated** using the same template engine as transformations.
This allows keys to depend on query params, path params, headers, and previous
step outputs.

```yaml
cache:
  backend: memory
  key: "user:{{ .query.userId }}:v{{ .user.version }}"
  ttl: 60s
```

### Stale-while-revalidate

When `staleWhileRevalidate: true`:

1. Cache hit → return immediately.
2. In background: refresh the entry if it's older than `ttl / 2`.
3. Background refresh uses a **detached context** with a fixed timeout so it
   doesn't block the request.

### Cache errors

Cache errors are **fail-open**: logged and counted, but never fail the
request. A cache outage degrades performance, not availability.

### Invalidation

Two mechanisms:

1. **TTL expiry**: each entry expires after its configured TTL.
2. **Explicit invalidation**: `cacheInvalidate` on write endpoints, or a
   `cache` step with `action: set` and a short TTL.

### Cache backends

| Backend | Latency | Shared | Use case |
|---------|---------|--------|----------|
| In-memory (`ristretto`) | ~100ns | No | Single instance, hot data |
| Redis | ~1ms | Yes | Multi-instance, shared state |

### Eviction

- **In-memory**: LRU eviction via `ristretto`, bounded by `maxSize` and
  `maxCost`.
- **Redis**: configured via `maxmemory-policy` on the Redis server.

---

## 11. Transformation Engine

Transformations use Go's `text/template` with custom functions.

### Compilation

Templates are **parsed once** at config load time:

```go
tmpl, err := template.New(name).Funcs(funcMap).Parse(templateString)
if err != nil {
    return fmt.Errorf("template %q: %w", name, err)
}
```

The parsed template is stored in the step. At request time, only `Execute` is
called.

### Execution

```go
var buf bytes.Buffer
buf.Grow(estimatedSize)

if err := step.tmpl.Execute(&buf, data); err != nil {
    return nil, fmt.Errorf("render %q: %w", step.ID, err)
}

return buf.Bytes(), nil
```

`buf` is obtained from a `sync.Pool` to reduce allocations.

### Data available to templates

| Variable | Source |
|----------|--------|
| `.query.*` | Query parameters |
| `.path.*` | Path parameters |
| `.header.*` | Request headers |
| `.body` | Parsed JSON body |
| `.bodyRaw` | Raw body string |
| `.requestID` | Request ID |
| `.<stepID>` | Output of a previous step |

### Why Go templates?

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Go `text/template` | Native, powerful, no deps | Steeper learning curve | **Chosen** |
| JMESPath | Simple extraction | Weak for shaping | Not chosen |
| JSONata | Powerful | Heavy dependency, own syntax | Not chosen |
| Custom DSL | Tailored | Maintenance burden | Not chosen |

### Safety

- Templates are **trusted** (provided by the operator).
- `env` function is available but documented as a security consideration.
- Template execution has a configurable timeout (default: 100ms per render).

---

## 12. Configuration Lifecycle

```
┌─────────────┐
│  Start up   │
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│  Load config file   │
│  - Read file        │
│  - Parse YAML/JSON  │
│  - Interpolate env  │
│  - Validate schema  │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│  Build engine       │
│  - Build upstreams  │
│  - Build caches     │
│  - Build steps      │
│  - Build pipelines  │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│  Start HTTP server  │
│  - Bind address     │
│  - Register routes  │
│  - Start admin srv  │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│  Watch config file  │
│  - fsnotify         │
│  - Debounce events  │
└─────────────────────┘
```

### Validation

Validation happens in two passes:

1. **Structural**: parse into structs, check types.
2. **Semantic**: check references (upstreams exist, caches exist, DAG is
   acyclic, templates parse).

Failures are collected and reported together, not one at a time.

### Why fail fast?

Elegba refuses to start with an invalid config. A misconfigured aggregator is
worse than a stopped one: it silently produces wrong responses.

---

## 13. Hot Reload

Hot reload swaps the engine atomically without dropping requests.

### Sequence

```
┌──────────────────┐
│  fsnotify event  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Debounce        │
│  (500ms default) │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Load new config │
│  Validate        │
└────────┬─────────┘
         │
    ┌────┴────┐
    │         │
 INVALID   VALID
    │         │
    ▼         ▼
 Log error  Build new engine
 keep old       │
                ▼
         ┌──────────────────┐
         │  atomic.Swap     │
         │  (new engine)    │
         └────────┬─────────┘
                  │
                  ▼
         ┌──────────────────┐
         │  Drain old       │
         │  (wait for       │
         │   in-flight)     │
         └────────┬─────────┘
                  │
                  ▼
         ┌──────────────────┐
         │  Close old       │
         │  (after timeout) │
         └──────────────────┘
```

### Atomic swap

```go
type Server struct {
    engine atomic.Pointer[Engine]
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    eng := s.engine.Load()
    eng.ServeHTTP(w, r)
}
```

No locks on the hot path. The swap is a single atomic pointer write.

### In-flight requests

Requests already executing on the old engine continue to completion. The old
engine's `Close()` waits for its `WaitGroup` to drain, up to
`shutdownTimeout`.

### What can be hot-reloaded

| Section | Hot reloadable? |
|---------|-----------------|
| `endpoints.*` | Yes |
| `upstreams.*` | Yes |
| `caches.*` | No (connection pools are long-lived) |
| `server.*` | No (requires restart) |

---

## 14. Observability Pipeline

Every request emits logs, metrics, and traces.

### Logs

Structured JSON via `log/slog`:

```json
{
  "time": "2026-01-15T10:30:00Z",
  "level": "INFO",
  "msg": "request completed",
  "request_id": "01HXYZ...",
  "endpoint": "/dashboard",
  "method": "GET",
  "status": 200,
  "duration_ms": 45,
  "steps": 3,
  "cache_hits": 1
}
```

Logs are emitted at:
- Request start (debug).
- Step start (debug).
- Step completion (debug).
- Request completion (info).
- Errors (error).

### Metrics

Prometheus metrics are registered at startup and updated per request.

| Metric | Type | Updated when |
|--------|------|--------------|
| `elegba_requests_total` | Counter | Request completes |
| `elegba_request_duration_seconds` | Histogram | Request completes |
| `elegba_step_duration_seconds` | Histogram | Step completes |
| `elegba_upstream_errors_total` | Counter | Upstream call fails |
| `elegba_cache_hits_total` | Counter | Cache hit |
| `elegba_cache_misses_total` | Counter | Cache miss |
| `elegba_circuit_breaker_state` | Gauge | Breaker state changes |
| `elegba_inflight_requests` | Gauge | Request start/end |

### Traces

OpenTelemetry spans:

```
Request span
├── Step: user (fetch)
│   └── Upstream call: user-service
├── Step: orders (fetch)
│   └── Upstream call: order-service
└── Step: combined (transform)
```

Trace context is propagated to upstreams via the `traceparent` header.

### Request ID

Every request gets a ULID at the middleware layer. It's:
- Injected into the context.
- Added to all log entries.
- Returned as `X-Request-ID` header.
- Propagated to upstreams as `X-Request-ID`.

---

## 15. Error Handling

Errors are categorized and handled consistently.

### Error categories

| Category | Example | HTTP | Behavior |
|----------|---------|------|----------|
| Config | Invalid YAML | 500 | Fail at startup |
| Routing | No endpoint match | 404 | Return structured error |
| Method | Path matches, method doesn't | 405 | Return structured error |
| Validation | Request body too large | 413 | Return structured error |
| Rate limit | Endpoint concurrency exceeded | 429 | Return structured error |
| Upstream timeout | `context.DeadlineExceeded` | 504 | Retry, then return |
| Upstream error | 5xx from upstream | 502 | Retry, then return |
| Breaker open | Circuit open | 503 | Fail fast |
| Transform | Template execution error | 500 | Return structured error |
| Cache | Redis connection error | — | Log, continue |
| Internal | Unexpected panic | 500 | Recover, log, return |

### Error response format

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

### Error wrapping

Errors are wrapped with context at each layer:

```go
if err != nil {
    return fmt.Errorf("step %q: fetch %q: %w", step.ID, upstream, err)
}
```

`errors.Is` and `errors.As` are used for sentinel checks.

### Panic recovery

A recovery middleware wraps the handler:

```go
defer func() {
    if r := recover(); r != nil {
        logger.Error("panic", "value", r, "stack", debug.Stack())
        metrics.IncPanics()
        http.Error(w, `{"error":{"code":"INTERNAL_ERROR"}}`, 500)
    }
}()
```

---

## 16. Graceful Shutdown

On SIGTERM or SIGINT:

```
┌──────────────────┐
│  Signal received │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Stop accepting  │
│  new requests    │
│  (srv.Shutdown)  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Drain in-flight │
│  requests        │
│  (up to timeout) │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Close upstream  │
│  HTTP clients    │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Close cache     │
│  connections     │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Flush tracing   │
│  (OTel)          │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Exit 0          │
└──────────────────┘
```

### Timeout

If in-flight requests don't complete within `shutdownTimeout` (default 30s),
they're forcibly cancelled and the process exits.

### Second signal

A second SIGTERM/SIGINT forces immediate exit.

---

## 17. Design Decisions & Trade-offs

### Why Go?

- Native concurrency for the DAG executor.
- Single static binary, minimal footprint.
- Fast startup, low memory.
- Rich standard library (HTTP, templates, JSON).
- Strong ecosystem for resilience and observability.

### Why YAML over JSON?

- Human-readable, comments allowed.
- Familiar to DevOps audiences.
- JSON is supported for programmatic generation.

### Why Go templates over JMESPath/JSONata?

- Native to Go, no external dependency.
- Powerful enough for shaping, not just extraction.
- Familiar to Go developers.
- Trade-off: less concise than JSONata for complex transformations.

### Why per-step caching instead of per-endpoint?

- Fine-grained control: cache the expensive call, not the cheap one.
- Cache keys can depend on previous step outputs.
- Trade-off: more config, more surface area.

### Why DAG over sequential pipeline?

- Parallelism by default.
- Explicit dependencies.
- Deterministic output.
- Trade-off: more complex than a list, but the config stays declarative.

### Why `failFast: false` as default?

- BFFs often want partial responses (user data even if orders fail).
- Operators can opt into strict mode with `failFast: true`.
- Trade-off: clients must handle `_errors` fields.

### Why atomic pointer for hot reload?

- Zero-cost on the hot path.
- No locks, no contention.
- Trade-off: old engine must be drained explicitly.

### Why no plugins in v1?

- Plugins (Go plugins, WASM) add complexity and security surface.
- The step registry already allows new step types via code.
- Plugins are planned for v2.

---

## 18. Extension Points

Elegba is designed to be extended. The following are the primary extension
points.

### Adding a new step type

1. Create `internal/step/mytype.go`.
2. Implement the `Step` interface.
3. Register with `step.Register("mytype", factory)` in `init()`.
4. Document in `config-reference.md`.

### Adding a new transport

1. Create `internal/transport/myproto.go`.
2. Implement the `Transport` interface.
3. Register with `transport.Register("myproto", factory)`.
4. Add config validation rules.

### Adding a new cache backend

1. Create `internal/cache/mybackend.go`.
2. Implement the `Cache` interface.
3. Register with `cache.Register("mybackend", factory)`.
4. Add config validation rules.

### Adding a new template function

1. Add the function to `internal/transform/funcs.go`.
2. Register it in the `FuncMap`.
3. Document in `config-reference.md`.

### Embedding Elegba

The `pkg/elegba` package exposes a public API for embedding:

```go
import "github.com/you/elegba/pkg/elegba"

eng, err := elegba.New(elegba.Config{
    Path: "elegba.yaml",
})
if err != nil {
    log.Fatal(err)
}

http.ListenAndServe(":8080", eng)
```

---

## 19. Diagrams

### 19.1 Full request flow

```
Client
  │
  ▼
HTTP Server ──▶ Middleware ──▶ Router ──▶ Executor ──▶ Response
  │                │              │           │
  │                │              │           ├──▶ Step: fetch
  │                │              │           │        │
  │                │              │           │        └──▶ Transport ──▶ Upstream
  │                │              │           │
  │                │              │           ├──▶ Step: fetch
  │                │              │           │        │
  │                │              │           │        └──▶ Transport ──▶ Upstream
  │                │              │           │
  │                │              │           └──▶ Step: transform
  │                │              │                    │
  │                │              │                    └──▶ Renderer
  │                │              │
  │                │              └──▶ Result Store
  │                │
  │                └──▶ Observability (logs, metrics, traces)
  │
  └──▶ X-Request-ID, response
```

### 19.2 DAG execution

```
Wave 1                Wave 2                Wave 3
┌──────┐  ┌──────┐   ┌──────────────┐      ┌──────────┐
│ user │  │orders│   │recommendation│      │ combined │
└──┬───┘  └──┬───┘   └──────┬───────┘      └────┬─────┘
   │         │              │                   │
   │         │              │                   │
   └─────────┴──────────────┴───────────────────┘
                concurrent execution
```

### 19.3 Resilience chain

```
Request
  │
  ▼
┌──────────────┐
│ Rate Limiter │──── 429 ──▶ Client
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Bulkhead   │──── timeout ──▶ Client
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Breaker    │──── open ──▶ 503 ──▶ Client
└──────┬───────┘
       │
       ▼
┌──────────────┐
│    Retry     │──── exhausted ──▶ 502 ──▶ Client
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Timeout    │──── deadline ──▶ 504 ──▶ Client
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  HTTP Client │────▶ Upstream
└──────────────┘
```

### 19.4 Hot reload

```
┌────────────┐        ┌────────────┐        ┌────────────┐
│  Watcher   │───────▶│  Loader    │───────▶│  Validator │
└────────────┘        └────────────┘        └─────┬──────┘
                                                   │
                                          ┌────────┴────────┐
                                          │                 │
                                       INVALID           VALID
                                          │                 │
                                          ▼                 ▼
                                   ┌────────────┐   ┌────────────┐
                                   │ Log error  │   │ Build new  │
                                   │ Keep old   │   │ engine     │
                                   └────────────┘   └─────┬──────┘
                                                          │
                                                          ▼
                                                   ┌────────────┐
                                                   │ atomic.Swap│
                                                   └─────┬──────┘
                                                         │
                                                         ▼
                                                   ┌────────────┐
                                                   │ Drain old  │
                                                   └────────────┘
```

---

*End of architecture document.*