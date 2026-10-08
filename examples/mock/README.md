# Mock upstreams

The mock services implement the user, ledger, and account endpoints used by the
user-summary example. Each service requires a non-empty `Authorization: Bearer`
header so the example demonstrates client-token forwarding.

Run from the repository root with:

```bash
go run ./examples/mock -address :8081 -service user
```

Use `ledger` and `account` for the other service identities.
