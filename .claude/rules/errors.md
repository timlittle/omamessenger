## Error Handling

- Return `error` as the last return value; check it immediately after the call
- Wrap with `fmt.Errorf("operation: %w", err)`; bare `return err` only when there is nothing useful to add
- Sentinel errors: `var ErrNotFound = errors.New("...")` at package level, in the package that owns the concept
- Check wrapped errors with `errors.Is`/`errors.As`; never compare `.Error()` strings
- Error strings: lowercase, no trailing punctuation, prefixed with the package or operation (`"store: open: …"`)
- Never discard errors with `_` in production code. If an error genuinely cannot matter (for example `Rollback` after a successful `Commit`), say why in a comment on that line
- No `log.Fatal` outside `main`; it calls `os.Exit` and skips deferred cleanup
- Panic only for programmer errors; recover at the protocol boundary so one bad request cannot crash the helper
- Errors that reach the UI carry a stable code and a safe message; internal details never leave the helper (see protocol.md)
