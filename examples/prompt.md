# Project: Elegba — Config-Driven API Aggregator & BFF

You are a senior Go engineer with deep expertise in building high-performance,
resilient network services. Your task is to build **Elegba**, a config-driven
API aggregator and Backend-for-Frontend (BFF) written in Go.

You must write **idiomatic, production-grade Go** that is:
- Performant (low latency, low allocations, efficient concurrency)
- Reliable (correct, well-tested, observable)
- Resilient (graceful degradation, timeouts, retries, circuit breaking)
- Maintainable (clean architecture, SOLID principles, clear boundaries)

Read the full specification in `DESIGN.md` before writing any code.

---

## 1. Language & Tooling Requirements

- **Go 1.22+** (use `log/slog`, `errors.Join`, generics where appropriate).
- **Module path**: `github.com/<you>/elegba`
- **Formatting**: `gofmt` + `goimports`.
- **Linting**: `golangci-lint` with a strict config (errcheck, govet, staticcheck,
  revive, gocritic, gosec, ineffassign, misspell, bodyclose, contextcheck,
  errorlint, exhaustive, nilerr, noctx, prealloc, unconvert, unparam).
- **Testing**: `go test` with `-race -cover`, table-driven tests, `testify`
  only where it improves clarity (prefer stdlib `testing` for simplicity).
- **Benchmarking**: `go test -bench=. -benchmem` for hot paths.
- **Documentation**: Every exported symbol must have a doc comment starting
  with the symbol name (godoc convention).

---

## 2. Architecture & Design Patterns

Apply these patterns deliberately, not gratuitously:

### 2.1 Clean Architecture / Hexagonal

- **Domain layer** (`internal/engine`, `internal/step`): pure business logic,
  no I/O, no framework imports.
- **Application layer** (`internal/config`, `internal/pipeline`): orchestration.
- **Infrastructure layer** (`internal/transport`, `internal/cache`,
  `internal/observability`): adapters to external systems.
- **Ports**: interfaces defined by the consumer, not the implementation
  (Go idiom: "accept interfaces, return structs").

### 2.2 Patterns to Use

- **Strategy Pattern**: `Transport`, `Cache`, `Step` are strategies selected
  from config at runtime.
- **Registry Pattern**: step types, transports, and cache backends self-register
  via `init()` or explicit registration, keyed by string. Use a factory
  function map: `map[string]func(Config) (Interface, error)`.
- **Builder Pattern**: for constructing complex `Pipeline` and `Upstream`
  objects from config.
- **Pipeline / Chain of Responsibility**: request → middleware → router →
  executor → steps.
- **DAG Executor**: topological sort of steps by `dependsOn`, execute
  independent nodes concurrently.
- **Functional Options**: for `NewServer`, `NewClient`, etc.
- **Dependency Injection**: constructor injection, no globals, no `init()`
  side effects beyond registration.
- **Interface Segregation**: small, focused interfaces (1–3 methods).
- **Context Propagation**: every I/O call takes `context.Context` first.
- **Result / Error types**: prefer returning errors over panics; use
  `errors.Is`/`errors.As` with sentinel errors and wrapped errors.

### 2.3 Anti-Patterns to Avoid

- No `interface{}` (use `any` only when truly needed, prefer generics).
- No goroutine leaks (always ensure goroutines exit on context cancel).
- No unbounded channels or slices (use semaphores and size limits).
- No `panic` in library code (only in `main` for fatal startup errors).
- No global mutable state.
- No silent error swallowing (`_ = doSomething()` is banned).
- No `time.Sleep` for synchronization (use channels/context).
- No `log.Fatal` outside `main`.
- No reflection in hot paths.

---

## 3. Concurrency Model — Critical Requirements

This is the heart of Elegba. Get it right.

- Use `golang.org/x/sync/errgroup` with `errgroup.WithContext` for pipeline
  execution.
- Use `golang.org/x/sync/semaphore` for per-upstream concurrency limits.
- Use `golang.org/x/time/rate` for per-upstream rate limiting.
- Every step execution must be wrapped in `context.WithTimeout`.
- Support **partial failure** via `failFast: false`:
  - Collect errors from failed steps into a `map[string]error` keyed by step ID.
  - Continue executing steps whose dependencies succeeded.
  - Return the aggregated response with a `_errors` field if any step failed.
- Ensure **no goroutine leaks**: use `defer cancel()`, wait on all goroutines
  before returning, and test with `goleak` in CI.
- Store step results in a concurrent-safe structure:
  - Prefer a `sync.Map` for reads-heavy access, or
  - A `map[string]any` guarded by `sync.RWMutex` with pre-sized capacity.
- **Determinism**: the final response shape must not depend on step completion
  order. Transform step reads from the results map, not from a channel.
- **Cancellation**: if `failFast: true` and one step fails, cancel the
  errgroup context immediately and let other steps observe cancellation.
- **Backpressure**: bound concurrency at the endpoint level with a
  configurable `maxConcurrent` (default: 100). Reject with 429 if exceeded.

Write a benchmark and a `goleak` test for the executor.

---

## 4. Performance Requirements

- **Zero-allocation hot paths** where feasible. Use `sync.Pool` for
  `bytes.Buffer`, `http.Request` bodies, and template execution contexts.
- **Pre-parse templates** at config load time, not per request.
- **Reuse HTTP clients** per upstream (one `*http.Client` per upstream,
  configured with its own `Transport`, timeouts, and connection pool).
- **Connection pooling**: set `MaxIdleConns`, `MaxIdleConnsPerHost`,
  `IdleConnTimeout` on each transport.
- **Avoid reflection** in request/response mapping; use codecs or
  compile-time structs.
- **Streaming**: for large upstream responses, stream with `io.Copy` instead
  of `io.ReadAll` where the transformation allows.
- **JSON**: use `encoding/json` for correctness; consider `goccy/go-json` or
  `jsoniter` only if benchmarks justify it. Do not use them by default.
- **Memory limits**: enforce `MaxResponseBytes` per upstream (default 10MB)
  via `http.MaxBytesReader`.
- **Benchmarks**: include `BenchmarkPipeline_2Steps`,
  `BenchmarkPipeline_10StepsParallel`, `BenchmarkTransform_Simple` with
  `-benchmem`, and document the results in `docs/benchmarks.md`.
- **Profiling**: expose `net/http/pprof` on a separate admin port (configurable,
  disabled by default).

---

## 5. Resilience Requirements

Every upstream call must be wrapped in a resilience chain:

```
[Rate Limiter] → [Circuit Breaker] → [Retry w/ Backoff] → [Timeout] → HTTP
```

- **Rate limiter**: `golang.org/x/time/rate`, per upstream.
- **Circuit breaker**: `sony/gobreaker` or `failsafe-go`. States: closed,
  open, half-open. Configurable thresholds.
- **Retries**: `cenkalti/backoff/v4` with exponential backoff + jitter.
  Retry only idempotent methods (GET, HEAD, OPTIONS) by default. Retry on:
  - Network errors
  - 5xx responses
  - 429 (respect `Retry-After`)
  - Do NOT retry on 4xx (except 429).
- **Timeouts**: two levels —
  - Per-upstream `timeout` (default 2s).
  - Per-request `timeout` at the endpoint level (default 10s).
- **Bulkhead**: per-upstream `maxConcurrent` via semaphore.
- **Fallback**: optional per-step `fallback` (static JSON or alternate
  upstream).
- **Graceful shutdown**: on SIGTERM/SIGINT, stop accepting new requests,
  drain in-flight requests up to `shutdownTimeout` (default 30s), then exit.
- **Health checks**: `/healthz` (process alive) and `/readyz` (all upstreams
  reachable within `readinessTimeout`).

Add chaos tests: simulate slow upstreams, 500s, timeouts, and verify behavior.

---

## 6. Observability Requirements

- **Logging**: `log/slog` with a JSON handler. Structured fields:
  `request_id`, `endpoint`, `step_id`, `upstream`, `duration_ms`, `status`.
  Log levels: `debug` (step I/O), `info` (request lifecycle), `warn`
  (retries, cache misses on critical paths), `error` (failures).
- **Metrics**: Prometheus via `prometheus/client_golang`. Expose:
  - `elegba_requests_total{endpoint, method, status}`
  - `elegba_request_duration_seconds{endpoint}` (histogram)
  - `elegba_step_duration_seconds{endpoint, step, upstream}` (histogram)
  - `elegba_upstream_errors_total{upstream, reason}`
  - `elegba_cache_hits_total{backend}`, `elegba_cache_misses_total{backend}`
  - `elegba_circuit_breaker_state{upstream}` (gauge: 0=closed, 1=half-open,
    2=open)
  - `elegba_inflight_requests{endpoint}` (gauge)
- **Tracing**: OpenTelemetry (`go.opentelemetry.io/otel`). One span per
  request, one child span per step, one child span per upstream call.
  Propagate `traceparent` headers to upstreams.
- **Request ID**: generate a ULID or UUIDv7 per request, inject into context,
  propagate as `X-Request-ID`.

---

## 7. Configuration Requirements

- **Format**: YAML primary, JSON supported. Use `gopkg.in/yaml.v3` and
  `encoding/json`.
- **Env var interpolation**: `${VAR}` and `${VAR:-default}` syntax. Resolve at
  load time; fail fast on missing required vars.
- **Validation**: `go-playground/validator/v10` with custom validators for
  durations, URLs, template syntax.
- **Schema versioning**: `version: "1"` field. Reject unknown versions.
- **Hot reload**: watch config file with `fsnotify`. On change:
  - Parse and validate the new config.
  - If invalid, log error and keep the old config running.
  - If valid, atomically swap the engine via `atomic.Pointer[Engine]`.
  - Drain in-flight requests from the old engine gracefully.
- **JSON Schema**: publish `schema/elegba.schema.json` for editor
  autocompletion.
- **Defaults**: every optional field must have a documented default. Use a
  `defaults.go` file with constants.
- **Strict parsing**: use `yaml.KnownFields(true)` (or `json.Decoder.DisallowUnknownFields()`)
  to reject unknown config keys. Users should not silently typo.

---

## 8. Security Requirements

- **TLS**: support both server-side TLS and per-upstream TLS configuration
  (CA bundle, client cert, `InsecureSkipVerify` off by default).
- **Secrets**: never log tokens, passwords, or API keys. Redact in
  structured logs and error messages.
- **Headers**: strip hop-by-hop headers (`Connection`, `Keep-Alive`, etc.)
  when proxying.
- **SSRF protection**: validate upstream URLs against a configurable allowlist
  if enabled.
- **Request size limits**: `http.MaxBytesReader` on incoming bodies
  (default 1MB).
- **Response size limits**: `MaxResponseBytes` per upstream.
- **CORS**: configurable allowlist, never `*` with credentials.
- **Timeouts**: prevent slowloris with `ReadHeaderTimeout`, `ReadTimeout`,
  `WriteTimeout`, `IdleTimeout` on the server.
- **Dependency scanning**: `govulncheck` in CI.

---

## 9. Project Layout (Strict)

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
│   ├── elegba.yaml
│   ├── dashboard.yaml
│   └── mobile-bff.yaml
├── docs/
│   ├── benchmarks.md
│   ├── config-reference.md
│   └── architecture.md
├── schema/
│   └── elegba.schema.json
├── testdata/
│   └── ...                        # golden files
├── DESIGN.md
├── README.md
├── CONTRIBUTING.md
├── CODE_OF_CONDUCT.md
├── LICENSE
├── Makefile
├── Dockerfile
├── .golangci.yml
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
└── go.mod
```

---

## 10. Testing Requirements

- **Coverage**: ≥ 85% on `internal/` packages. Enforce in CI.
- **Table-driven tests** for all pure logic (DAG, config, transform).
- **Integration tests** using `httptest.Server` for upstream mocking.
- **Golden file tests** for transformation outputs (`testdata/*.golden`).
- **Race detector**: all tests run with `-race`.
- **Goroutine leak detection**: `go.uber.org/goleak` in `TestMain`.
- **Fuzz tests**: `FuzzConfigLoader`, `FuzzTemplateRender`.
- **Benchmarks**: for executor, transform, transport.
- **Chaos tests**: slow upstreams, 500s, timeouts, connection resets.
- **Contract tests**: verify that the transformed response matches the config
  spec for a given input.

---

## 11. Build & Deployment

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

## 12. Documentation Requirements

- **README.md**: quick start, install, first config, link to docs.
- **DESIGN.md**: full architecture (already provided).
- **docs/config-reference.md**: every field, its type, default, and example.
- **docs/architecture.md**: diagrams, concurrency model, DAG execution.
- **docs/benchmarks.md**: methodology and results.
- **Godoc**: every exported symbol documented.
- **Examples**: at least 3 real-world configs (dashboard, mobile BFF,
  microservice composition).

---

## 13. Delivery Order

Build in this order, committing after each milestone:

1. **Scaffold**: module, layout, Makefile, CI, linter config.
2. **Config**: structs, loader, validation, defaults, env interpolation.
3. **Step interface + registry**.
4. **DAG executor** with concurrency, cancellation, partial failure.
5. **Fetch step** + HTTP transport (no resilience yet).
6. **Transform step** with Go templates + custom funcs.
7. **Cache step** + in-memory backend.
8. **Resilience**: retries, circuit breaker, rate limiter, timeouts.
9. **Observability**: logging, metrics, tracing, health checks.
10. **Hot reload**.
11. **Redis cache**.
12. **Docs, examples, benchmarks, chaos tests**.
13. **Release**: Dockerfile, goreleaser, SBOM.

At each step, run `go test -race ./...`, `golangci-lint run`, and
`govulncheck ./...` before proceeding.

---

## 14. Definition of Done

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

---

## 15. What to Output

For each milestone, output:

1. **A short plan** (3–5 bullets) of what you'll build.
2. **The full file contents** for every new or changed file, with the file
   path as a header.
3. **The exact commands** to verify (test, lint, bench).
4. **A one-paragraph summary** of design decisions and trade-offs.

Do not skip tests. Do not skip error handling. Do not use `panic` outside
`main`. Do not use `interface{}` when `any` or generics suffice. Do not
introduce a dependency without justifying it.

Begin with Milestone 1: Scaffold.