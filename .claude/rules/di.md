## Dependency Injection

- Wire all dependencies in `backend/main.go`; business packages never construct their own infrastructure
- Business logic receives its dependencies from outside, as interfaces
- No global variables or package-level singletons for dependencies or mutable state; they prevent parallel tests and hide coupling
- No `init()` functions for setup
- Infrastructure adapters (store, connectors, server) import `domain`; `domain` imports nothing internal
- Allowed internal imports per package are enforced by depguard in `.golangci.yml`; a new package needs a rule there
