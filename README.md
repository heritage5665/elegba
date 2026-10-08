# README.md

# Elegba

Elegba is a config-driven API aggregator and Backend-for-Frontend (BFF) written in Go. Its MVP serves configured HTTP endpoints, concurrently fetches upstream data, and shapes the result with Go templates.

## Features

- **Config-Driven**: Easily configure the application using YAML or JSON.
- **High Performance**: Built with Go, ensuring low latency and efficient concurrency.
- **Concurrent pipelines**: Independent upstream calls run in parallel and dependencies are validated at startup.
- **Resilience**: Per-upstream retries with jitter and `Retry-After`, circuit breakers, token-bucket rate limits, and request concurrency limits.
- **Graceful degradation**: Per-step timeouts, partial results with `_errors`, and ignore/static/upstream/cache fallbacks.
- **Transforms and caching**: Go templates shape output; named Ristretto or Redis caches support fetch cache-through, stale refresh, explicit get/set/delete steps, and endpoint invalidation.
- **Observability**: Prometheus metrics, optional OTLP traces, traceparent propagation, request IDs, and secret-redacted JSON logs.
- **Operations**: Optional private admin listener for metrics/pprof, active readiness probes, health checks, and graceful shutdown.

## Getting Started

### Prerequisites

- Go 1.22 or higher
- Make

### Installation

1. Clone the repository:

   ```
   git clone https://github.com/heritage5665/elegba.git
   cd elegba
   ```

2. Build the application:

   ```
   make build
   ```

### Running the Application

To run the application, use the following command:

```
make run
```

The example listens on `:8080`; try `http://localhost:8080/dashboard?id=1`. Set `CONFIG=path/to/config.yaml` to use another file.

### Client Auth Worked Example

The canonical `Client Auth & Worked Example` is runnable with Docker Compose:

```bash
docker compose up --build
curl -H 'Authorization: Bearer demo-token' \
  'http://localhost:8080/users/42/summary'
```

This starts Elegba plus mock user, ledger, and account services. See
[docs/examples/user-summary.md](docs/examples/user-summary.md) for the complete
walkthrough, expected response, cache safety rule, and failure modes.

### Configuration

Configuration is strict YAML or JSON. Unknown fields and invalid references are rejected at startup. `${VAR}` requires a non-empty environment variable; `${VAR:-default}` supplies a fallback. The runnable example is in [examples/elegba.yaml](examples/elegba.yaml), and the client-auth worked example is in [examples/user-summary.yaml](examples/user-summary.yaml). Supported fields are documented in [docs/config-reference.md](docs/config-reference.md).

### Documentation

For detailed documentation, please refer to the following files:

- [DESIGN.md](DESIGN.md): Design decisions and architectural patterns.
- [docs/architecture.md](docs/architecture.md): Overview of the application architecture.
- [schema/elegba.schema.json](schema/elegba.schema.json): JSON schema for configuration validation.

## Contributing

Contributions are welcome! Please read the [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute to the project.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.