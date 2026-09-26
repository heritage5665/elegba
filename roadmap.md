# Elegba — Roadmap

**Version:** 1.0
**Applies to:** Elegba v1.x → v2.x
**Status:** Living document

This document describes the delivery roadmap for Elegba: what will be built,
in what order, and why. It is organized into milestones, each with clear
deliverables, acceptance criteria, and out-of-scope items.

The roadmap is a **guide, not a contract**. Priorities may shift based on
community feedback, security issues, or upstream dependencies.

---

## Table of Contents

1. [Guiding Principles](#1-guiding-principles)
2. [Release Cadence](#2-release-cadence)
3. [Versioning Policy](#3-versioning-policy)
4. [Milestone 0 — Foundation](#4-milestone-0--foundation)
5. [Milestone 1 — MVP](#5-milestone-1--mvp)
6. [Milestone 2 — Resilience](#6-milestone-2--resilience)
7. [Milestone 3 — Observability](#7-milestone-3--observability)
8. [Milestone 4 — Caching](#8-milestone-4--caching)
9. [Milestone 5 — Hot Reload](#9-milestone-5--hot-reload)
10. [Milestone 6 — v1.0 Release](#10-milestone-6--v10-release)
11. [Milestone 7 — v1.x Hardening](#11-milestone-7--v1x-hardening)
12. [Milestone 8 — v2.0 Planning](#12-milestone-8--v20-planning)
13. [Beyond v2.0](#13-beyond-v20)
14. [Explicitly Out of Scope](#14-explicitly-out-of-scope)
15. [How to Contribute](#15-how-to-contribute)

---

## 1. Guiding Principles

The roadmap follows these principles:

1. **Working software over comprehensive features.** Each milestone ships
   something usable.
2. **Correctness before performance.** Get it right, then make it fast.
3. **Observability from day one.** No feature ships without logs, metrics, and
   traces.
4. **No feature without tests.** Coverage ≥ 85% on `internal/`.
5. **No feature without docs.** Every field, every flag, every error.
6. **Small, reviewable changes.** Each PR is a single logical change.
7. **Backwards compatibility.** Config schema changes require a version bump.
8. **Security is not a milestone.** It is a continuous requirement.

---

## 2. Release Cadence

| Release type | Cadence | Notes |
|--------------|---------|-------|
| Patch (v1.0.x) | As needed | Security fixes, critical bugs. |
| Minor (v1.x.0) | Every 4–6 weeks | New features, non-breaking changes. |
| Major (vX.0.0) | Every 6–12 months | Breaking changes, schema version bump. |

All releases are tagged in Git, signed, and published with:
- Binaries for Linux, macOS, Windows (amd64, arm64).
- Docker images (multi-arch).
- SBOM (CycloneDX).
- Checksums (SHA-256).
- Release notes.

---

## 3. Versioning Policy

Elegba follows **Semantic Versioning 2.0.0**.

| Component | Follows semver? |
|-----------|-----------------|
| Binary / module | Yes |
| Config schema | Yes (via `version` field) |
| Public Go API (`pkg/elegba`) | Yes |
| Internal packages (`internal/`) | No (private) |

### Config schema versioning

- The config schema is versioned independently via the `version` field.
- Adding optional fields is **non-breaking** (same version).
- Removing or renaming fields is **breaking** (new version).
- Renaming a field requires a migration path in `docs/config-reference.md`.

### Deprecation

Deprecated features are:
1. Announced in release notes.
2. Logged as warnings at runtime.
3. Documented in `docs/config-reference.md`.
4. Removed after **two minor releases** or **one major release**.

---

## 4. Milestone 0 — Foundation

**Goal:** A working skeleton that builds, tests, and lints.

**Status:** ✅ Complete (assumed for v1 planning)

### Deliverables

- [x] Go module initialized (`github.com/<you>/elegba`).
- [x] Project layout per `DESIGN.md` §15.
- [x] `Makefile` with `build`, `test`, `lint`, `bench`, `cover`, `docker`.
- [x] `.golangci.yml` with strict linters.
- [x] GitHub Actions CI: lint, test, `govulncheck`.
- [x] `cmd/elegba/main.go` with flag parsing and structured logging.
- [x] `LICENSE` (MIT), `README.md`, `DESIGN.md`.
- [x] `.gitignore`, `.editorconfig`.
- [x] `schema/elegba.schema.json` (draft 2020-12).

### Acceptance Criteria

- `make build` produces a binary.
- `make test` passes with `-race`.
- `make lint` passes with zero warnings.
- CI is green on `main`.

### Out of Scope

- Any business logic.
- Any HTTP handling beyond a stub.

---

## 5. Milestone 1 — MVP

**Goal:** A working aggregator that can fetch from multiple upstreams
concurrently and transform the response.

**Target:** v0.1.0 (pre-release)

**Status:** Implementation complete. Race-detector and golangci-lint checks are
pending an environment with CGO and golangci-lint installed.

### Deliverables

#### Config

- [x] Config structs (`internal/config/config.go`).
- [x] YAML loader with env interpolation (`${VAR}`, `${VAR:-default}`).
- [x] Validator with clear error messages.
- [x] Defaults (`internal/config/defaults.go`).
- [x] Strict parsing (unknown keys rejected).

#### Engine

- [x] `Engine` type with `http.Handler` interface.
- [x] `Router` matching `(method, path)` to endpoints.
- [x] Path parameter extraction (`/users/{id}`).
- [x] Query/header/body injection into context.

#### Pipeline

- [x] `Step` interface.
- [x] `StepRegistry` with factory map.
- [x] DAG builder + cycle detection.
- [x] Topological sort (Kahn's algorithm).
- [x] Executor with `errgroup`, waves, `failFast` support.
- [x] Result store with `sync.RWMutex`.

#### Steps

- [x] `fetch` step with HTTP transport.
- [x] `transform` step with Go templates.
- [x] Built-in template functions: `toJSON`, `fromJSON`, `len`, `index`,
      `default`, `add`, `sum`.

#### Transport

- [x] `Transport` interface.
- [x] HTTP transport with `net/http`.
- [x] Basic auth (bearer, basic, apikey).
- [x] Connection pooling.

#### Observability

- [x] `log/slog` structured JSON logging.
- [x] Request ID (ULID) middleware.
- [x] `/healthz` and `/readyz` endpoints.

#### Cache

- [x] `Cache` interface.
- [x] In-memory backend (`ristretto`).
- [x] `cache` step (get/set).

#### CLI

- [x] `--config` flag.
- [x] `--log-level` flag.
- [x] Graceful shutdown on SIGTERM/SIGINT.

#### Tests

- [x] Unit tests for config, DAG, executor, steps.
- [x] Integration tests with `httptest.Server`.
- [x] Golden file tests for transformations.
- [x] Coverage ≥ 80% on `internal/` (80.6%).

### Acceptance Criteria

- A user can write a YAML config, run `elegba --config elegba.yaml`, and get
  a transformed response from multiple upstreams.
- All tests pass with `-race`.
- Lint is clean.
- No goroutine leaks (verified by `goleak`).

### Out of Scope

- Resilience (retries, breakers, rate limits).
- Redis cache.
- Prometheus metrics, OpenTelemetry tracing.
- Hot reload.
- Partial failure handling (`failFast: false` is a no-op).

### Example Config (MVP)

```yaml
version: "1"
upstreams:
  user:
    baseURL: https://api.example.com
    auth:
      type: bearer
      token: ${USER_TOKEN}
endpoints:
  - path: /dashboard
    method: GET
    pipeline:
      - id: user
        type: fetch
        upstream: user
        path: /users/{{ .query.id }}
      - id: combined
        type: transform
        dependsOn: [user]
        template: |
          {"user": {{ .user | toJSON }}}
```

---

## 6. Milestone 2 — Resilience

**Goal:** Elegba survives upstream failures gracefully.

**Target:** v0.2.0

**Status:** Implementation complete.

### Deliverables

#### Retries

- [x] `retry` config per upstream.
- [x] Exponential backoff with jitter (`cenkalti/backoff/v4`).
- [x] Retry only idempotent methods (GET, HEAD, OPTIONS).
- [x] Respect `Retry-After` on 429.
- [x] Retry budget (`maxElapsedTime`).

#### Circuit Breaker

- [x] `breaker` config per upstream.
- [x] Three states: closed, open, half-open (`sony/gobreaker`).
- [x] Configurable thresholds.
- [x] Emit state transition events.

#### Rate Limiting

- [x] `rateLimit` config per upstream (`golang.org/x/time/rate`).
- [x] Per-endpoint concurrency limit (`maxConcurrent`).
- [x] 429 response with `Retry-After` header.

#### Timeouts

- [x] Per-upstream `timeout`.
- [x] Per-endpoint `timeout`.
- [x] Per-step timeout (derived).
- [x] Slowloris protection (`ReadHeaderTimeout`, `ReadTimeout`,
      `WriteTimeout`, `IdleTimeout`).

#### Partial Failure

- [x] `failFast: false` collects errors per step.
- [x] `_errors` field in response when partial failures occur.
- [x] `onError: ignore` and `onError: fallback` per step.
- [x] Fallback types: `static`, `upstream`, `cacheKey`.

#### Errors

- [x] Structured error responses with codes.
- [x] Sentinel errors for `errors.Is`/`errors.As`.
- [x] Error wrapping with context at each layer.

#### Tests

- [x] Chaos tests: slow upstreams, 500s, timeouts, connection resets.
- [x] Breaker state transition tests.
- [x] Retry exhaustion tests.
- [x] Partial failure integration tests.

### Acceptance Criteria

- Upstream timeouts do not cascade.
- Breaker opens after threshold, recovers after timeout.
- Rate limits are enforced per upstream.
- Partial failures return usable responses.
- All chaos tests pass.

### Out of Scope

- Prometheus metrics, OpenTelemetry tracing.
- Redis cache.
- Hot reload.

---

## 7. Milestone 3 — Observability

**Goal:** Full observability for production operations.

**Target:** v0.3.0

**Status:** Implementation complete.

### Deliverables

#### Metrics

- [x] Prometheus integration (`prometheus/client_golang`).
- [x] `/metrics` endpoint on admin server.
- [x] Metrics:
  - `elegba_requests_total{endpoint, method, status}`
  - `elegba_request_duration_seconds{endpoint}` (histogram)
  - `elegba_step_duration_seconds{endpoint, step, upstream}` (histogram)
  - `elegba_upstream_errors_total{upstream, reason}`
  - `elegba_cache_hits_total{backend}`, `elegba_cache_misses_total{backend}`
  - `elegba_circuit_breaker_state{upstream}`
  - `elegba_inflight_requests{endpoint}`

#### Tracing

- [x] OpenTelemetry integration (`go.opentelemetry.io/otel`).
- [x] OTLP exporter (configurable endpoint).
- [x] Span per request, per step, per upstream call.
- [x] `traceparent` propagation to upstreams.

#### Logging

- [x] Structured logging with `log/slog`.
- [x] Correlation IDs (`request_id`) in every log.
- [x] Log levels: `debug`, `info`, `warn`, `error`.
- [x] Redaction of secrets (tokens, passwords, API keys).

#### Admin Server

- [x] Separate admin port (`:9090` by default, disabled by default).
- [x] `/metrics`, `/debug/pprof/*`, `/healthz`, `/readyz`.
- [x] Configurable via `server.admin`.

#### Health Checks

- [x] `/healthz` — liveness (process alive).
- [x] `/readyz` — readiness (all upstreams reachable within timeout).
- [x] Configurable `readinessTimeout`.

#### Request ID

- [x] ULID or UUIDv7 per request.
- [x] Injected into context.
- [x] Propagated as `X-Request-ID` to upstreams.
- [x] Returned in response headers.

#### Tests

- [x] Metrics tests (counter increments, histogram buckets).
- [x] Tracing tests (span hierarchy and OTLP export).
- [x] Log format tests (JSON structure and redaction).

### Acceptance Criteria

- Prometheus can scrape `/metrics`.
- Jaeger/Tempo can ingest traces via OTLP.
- Logs are parseable JSON with consistent fields.
- Health checks reflect actual readiness.

### Out of Scope

- Redis cache.
- Hot reload.

---

## 8. Milestone 4 — Caching

**Goal:** Flexible, multi-backend caching.

**Target:** v0.4.0

**Status:** Implementation complete.

### Deliverables

#### Cache Interface

- [x] `Cache` interface with context-aware `Get`, `Set`, `Delete`, `Close`.
- [x] Cache registry with factory map.
- [x] Cache errors are fail-open (logged, not fatal).

#### Backends

- [x] In-memory backend (`ristretto`) — already in MVP.
- [x] Redis backend (`go-redis/v9`).
- [x] Connection pooling for Redis.
- [x] TLS support for Redis.

#### Step-Level Caching

- [x] `cache` config on `fetch` steps.
- [x] Templated cache keys.
- [x] Per-step TTL override.
- [x] `staleWhileRevalidate` support.

#### Cache Step

- [x] Explicit `cache` step with `action: get`, `set`, or `delete`.
- [x] Use for invalidation on write endpoints.

#### Invalidation

- [x] `cacheInvalidate` on endpoints (before pipeline).
- [x] `cache` step with `set` + short TTL for manual invalidation.

#### Tests

- [x] Cache hit/miss tests.
- [x] TTL expiry tests.
- [x] Stale-while-revalidate tests.
- [x] Redis integration tests (via Miniredis).
- [x] Cache failure (fail-open) tests.

### Acceptance Criteria

- In-memory and Redis caches both work.
- Cache keys are correctly templated.
- TTL is respected.
- Cache failures never fail requests.
- Redis integration tests pass in CI.

### Out of Scope

- Memcached backend (v2).
- Distributed cache coherence (v2).
- Hot reload of cache backends.

---

## 9. Milestone 5 — Hot Reload

**Goal:** Reload config without dropping requests.

**Target:** v0.5.0

### Deliverables

#### Config Watcher

- [ ] `fsnotify`-based file watcher.
- [ ] Debounce events (default 500ms).
- [ ] Reload on file change.
- [ ] Ignore non-config files.

#### Atomic Swap

- [ ] `atomic.Pointer[Engine]` for engine swap.
- [ ] Zero-cost on the hot path.
- [ ] New engine built before swap.

#### Drain

- [ ] Old engine drains in-flight requests.
- [ ] Drain timeout configurable.
- [ ] Forceful shutdown after timeout.

#### Validation

- [ ] Invalid configs are rejected, old config continues running.
- [ ] Errors logged with clear messages.
- [ ] Metrics for reload success/failure.

#### Configuration

- [ ] `ELEGBA_HOT_RELOAD` env var (default `true`).
- [ ] `ELEGBA_HOT_RELOAD_DEBOUNCE` env var (default `500ms`).

#### Tests

- [ ] Hot reload integration tests.
- [ ] Invalid config does not break running server.
- [ ] In-flight requests complete after reload.
- [ ] No goroutine leaks during reload.

### Acceptance Criteria

- Changing the config file reloads endpoints and upstreams.
- In-flight requests complete on the old engine.
- Invalid configs are rejected gracefully.
- No downtime during reload.

### Out of Scope

- Hot reload of `server.*` (requires restart).
- Hot reload of `caches.*` (connection pools are long-lived).

---

## 10. Milestone 6 — v1.0 Release

**Goal:** Production-ready v1.0.

**Target:** v1.0.0

### Deliverables

#### Documentation

- [ ] `README.md` — quick start, install, first config.
- [ ] `DESIGN.md` — full architecture.
- [ ] `docs/config-reference.md` — every field documented.
- [ ] `docs/architecture.md` — internal design.
- [ ] `docs/benchmarks.md` — benchmark methodology and results.
- [ ] `docs/roadmap.md` — this document.
- [ ] `CONTRIBUTING.md` — dev setup, PR checklist.
- [ ] `CODE_OF_CONDUCT.md` — Contributor Covenant.
- [ ] `SECURITY.md` — vulnerability reporting.
- [ ] `CHANGELOG.md` — release history.

#### Examples

- [ ] `examples/elegba.yaml` — minimal config.
- [ ] `examples/dashboard.yaml` — BFF dashboard pattern.
- [ ] `examples/mobile-bff.yaml` — mobile BFF pattern.
- [ ] `examples/microservice-composition.yaml` — service composition.

#### Deployment

- [ ] `Dockerfile` — multi-stage, distroless, non-root, < 20MB.
- [ ] `docker-compose.yml` — local dev stack (Elegba + Redis + Prometheus).
- [ ] Helm chart — `deploy/helm/elegba/`.
- [ ] Kubernetes manifests — `deploy/k8s/`.
- [ ] `.goreleaser.yml` — multi-platform builds.

#### CI/CD

- [ ] GitHub Actions: lint, test, `govulncheck`, build.
- [ ] Release workflow: tag → build → publish binaries + Docker + SBOM.
- [ ] Coverage gate (≥ 85% on `internal/`).
- [ ] Benchmark regression check.

#### Security

- [ ] `govulncheck` in CI.
- [ ] Dependency pinning (`go.sum` committed).
- [ ] SBOM generation (Syft).
- [ ] Security policy published.

#### Performance

- [ ] Benchmarks published in `docs/benchmarks.md`.
- [ ] Zero-allocation hot paths where feasible.
- [ ] Profiling endpoints documented.

#### Tests

- [ ] Coverage ≥ 85% on `internal/`.
- [ ] All chaos tests pass.
- [ ] All integration tests pass.
- [ ] `goleak` in `TestMain`.

### Acceptance Criteria

- A user can install Elegba via Docker or binary, write a config, and serve
  aggregated APIs in production.
- All documentation is complete and accurate.
- All CI checks pass.
- No known security vulnerabilities.
- Benchmarks are reproducible.

### Out of Scope

- gRPC transport.
- Conditional/loop steps.
- Plugin system.
- Admin UI.

---

## 11. Milestone 7 — v1.x Hardening

**Goal:** Stabilize v1.x with community feedback.

**Target:** v1.1.0 → v1.x.0

### Deliverables (rolling)

#### Feedback-Driven

- [ ] Bug fixes from GitHub issues.
- [ ] Performance improvements from profiling.
- [ ] Documentation improvements.
- [ ] Additional examples.

#### Small Features

- [ ] Additional template functions (based on demand).
- [ ] Additional auth types (e.g., AWS SigV4).
- [ ] Additional cache backends (e.g., Memcached).
- [ ] Response compression (gzip, brotli).
- [ ] Request/response body logging (opt-in).
- [ ] Config linting CLI (`elegba lint --config ...`).
- [ ] Config diff CLI (`elegba diff old.yaml new.yaml`).

#### Operations

- [ ] Readiness probe tuning.
- [ ] Graceful degradation modes.
- [ ] Runtime config inspection endpoint (admin).
- [ ] Debug endpoints (per-endpoint, per-upstream status).

#### Security

- [ ] SSRF allowlist.
- [ ] Request size limits tuning.
- [ ] Secret redaction improvements.
- [ ] Audit logging.

### Acceptance Criteria

- v1.x releases are backwards compatible.
- No regressions in performance.
- Community feedback is addressed in a timely manner.

---

## 12. Milestone 8 — v2.0 Planning

**Goal:** Plan the next major version.

**Target:** v2.0.0

### Candidate Features

These are **candidates**, not commitments. Prioritization depends on community
demand.

#### Transports

- [ ] gRPC transport.
- [ ] GraphQL transport.
- [ ] WebSocket support.

#### Steps

- [ ] `conditional` step (if/else).
- [ ] `loop` step (iterate over collection).
- [ ] `parallel` step (explicit fan-out).
- [ ] `map` step (transform each element).
- [ ] `assert` step (validate response shape).

#### Plugin System

- [ ] Go plugin support (`plugin` package).
- [ ] WASM plugin support (`wazero`).
- [ ] Plugin sandboxing.
- [ ] Plugin registry.

#### Admin UI

- [ ] Read-only config viewer.
- [ ] Endpoint/upstream status dashboard.
- [ ] Metrics visualization.
- [ ] Config diff viewer.

#### Advanced Caching

- [ ] Cache warming.
- [ ] Distributed cache coherence.
- [ ] Cache tags for group invalidation.
- [ ] Negative caching.

#### Advanced Resilience

- [ ] Adaptive concurrency limits.
- [ ] Load shedding.
- [ ] Hedged requests.
- [ ] Request prioritization.

#### Multi-Tenancy

- [ ] Tenant isolation.
- [ ] Per-tenant quotas.
- [ ] Per-tenant config overlays.

#### Deployment

- [ ] Operator for Kubernetes.
- [ ] Service mesh integration (Istio, Linkerd).
- [ ] Edge deployment (Cloudflare Workers, Fly.io).

### Breaking Changes Under Consideration

- Config schema v2 with improved ergonomics.
- Public API (`pkg/elegba`) redesign.
- Metric name changes.
- Default behavior changes.

All breaking changes will be documented in a migration guide.

---

## 13. Beyond v2.0

Long-term ideas, not commitments:

- **Multi-region aggregation** — fan out across regions.
- **Streaming responses** — SSE, chunked responses.
- **Schema stitching** — GraphQL federation.
- **AI-assisted config** — generate config from OpenAPI specs.
- **Config marketplace** — shareable config templates.
- **Managed cloud offering** — hosted Elegba.
- **Language bindings** — embed Elegba in other languages via CGO.

These are exploratory and will be evaluated based on community interest.

---

## 14. Explicitly Out of Scope

The following are **not** planned for Elegba, at any version:

- **Full API gateway features** — auth server, developer portal, billing,
  rate limiting at the edge beyond per-upstream limits.
- **Service mesh** — sidecar injection, mTLS orchestration, traffic shaping.
- **Durable workflow orchestration** — long-running stateful workflows,
  sagas, compensation.
- **UI-driven configuration** — a graphical config editor.
- **Persistent state** — Elegba is stateless by design.
- **Multi-tenant isolation** — one process per tenant.
- **Custom DSL** — config is YAML/JSON, not a new language.

If you need these, consider KrakenD, Kong, or a full API gateway.

---

## 15. How to Contribute

Contributions are welcome at any milestone.

### Where to start

- **Good first issues** — labeled `good first issue` on GitHub.
- **Help wanted** — labeled `help wanted` on GitHub.
- **Documentation** — always appreciated.
- **Examples** — real-world configs are valuable.

### How to propose a feature

1. Open a GitHub issue with the `feature request` label.
2. Describe the use case, not just the solution.
3. Wait for maintainer feedback before implementing.
4. If accepted, follow the `CONTRIBUTING.md` guide.

### How to propose a breaking change

1. Open a GitHub issue with the `breaking change` label.
2. Describe the motivation, migration path, and impact.
3. Breaking changes require a major version bump and a migration guide.

### How to report a bug

1. Open a GitHub issue with the `bug` label.
2. Include: Elegba version, config, expected behavior, actual behavior, logs.
3. Minimal reproducible example is greatly appreciated.

### How to report a security issue

See `SECURITY.md`. Do not open a public issue.

---

## Appendix A: Milestone Summary

| Milestone | Version | Focus | Status |
|-----------|---------|-------|--------|
| 0 | — | Foundation | ✅ Complete |
| 1 | v0.1.0 | MVP | 🚧 In progress |
| 2 | v0.2.0 | Resilience | ⏳ Planned |
| 3 | v0.3.0 | Observability | ⏳ Planned |
| 4 | v0.4.0 | Caching | ⏳ Planned |
| 5 | v0.5.0 | Hot Reload | ⏳ Planned |
| 6 | v1.0.0 | Release | ⏳ Planned |
| 7 | v1.x | Hardening | ⏳ Planned |
| 8 | v2.0.0 | Next major | 💭 Exploratory |

## Appendix B: Dependency Additions per Milestone

| Milestone | New dependencies |
|-----------|------------------|
| 1 | `gopkg.in/yaml.v3`, `golang.org/x/sync`, `github.com/dgraph-io/ristretto`, `github.com/go-playground/validator/v10`, `github.com/oklog/ulid/v2`, `github.com/oliveagle/jsonpath` |
| 2 | `github.com/cenkalti/backoff/v4`, `github.com/sony/gobreaker`, `golang.org/x/time/rate` |
| 3 | `github.com/prometheus/client_golang`, `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` |
| 4 | `github.com/redis/go-redis/v9` |
| 5 | `github.com/fsnotify/fsnotify` |
| 6 | `github.com/goreleaser/goreleaser` (dev), `github.com/anchore/syft` (dev) |

## Appendix C: Definition of Done (per milestone)

A milestone is done when:

- [ ] All deliverables are implemented.
- [ ] All acceptance criteria are met.
- [ ] Coverage ≥ 85% on `internal/`.
- [ ] All linters pass.
- [ ] `govulncheck` is clean.
- [ ] No goroutine leaks (`goleak`).
- [ ] No data races (`-race`).
- [ ] Documentation is updated.
- [ ] At least one example demonstrates the new feature.
- [ ] Release notes are written.
- [ ] Binary and Docker image are published.

---

*End of roadmap.*