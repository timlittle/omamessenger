## Testing (Go)

### TDD

- Write a failing test before the production code. Run it and confirm it fails for the expected reason
- Write the minimum code to pass, then refactor while green
- A bug fix starts with a test that reproduces the bug

### Layout

- Tests mirror source files: `store/messages.go` → `store/messages_test.go`. No grab-bag files named after a technique (`failure_test.go`, `frozen_test.go`, `fixture_test.go`); a failure-path test lives beside the happy path for the same function
- Shared test helpers for a package go in `helpers_test.go`, and each one calls `t.Helper()`
- Fuzz tests sit in the mirrored file (`FuzzDecodeRequest` in `server/codec_test.go`)
- Prefer black-box tests (`package store_test`). Use an internal test only when the behaviour cannot be reached through the public API, and say why in a comment
- No `export_test.go` back doors; if a test needs it, the API is wrong
- A fake shared across packages lives in a `<pkg>test` package (`connectortest.Clock`); otherwise it is unexported in the test file that uses it

### Style

- Table-driven tests with named cases and `t.Run`; use `t.Parallel()` where the test has no shared state
- Test names describe behaviour: `TestSend_RejectsEmptyText`, not `TestSend2`
- Hand-written fakes. No mocking frameworks
- More than 3 test doubles in one test means the unit has too many dependencies: redesign it
- Use real SQLite in a `t.TempDir()`, not a fake store
- Use `t.Context()`, never `context.Background()`, in tests
- No `time.Sleep`; use the fake clock or synchronise on channels
- Assert on behaviour and returned values, not on internal calls

### Gates

- `make check` runs everything; it must pass before a change is done
- Coverage gates live only in `.testcoverage.yml` (90 % per package by default). Do not lower a gate to pass; add the missing tests
