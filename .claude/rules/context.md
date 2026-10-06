## Context Usage

- First argument of every function that does I/O or may block: `ctx context.Context`
- Never store a context in a struct; pass it through the call chain
- `context.Value` only for cross-cutting metadata, never for inputs or dependencies
- Use `context.WithTimeout` for operations that must not run forever (network calls in connectors, downloads)
- Never pass `nil` as a context; use `context.Background()` in `main` and `t.Context()` in tests
- The helper's root context is cancelled on SIGTERM or when stdin closes; everything it starts must stop with it
