## Concurrency

- `go test -race` always; the race detector must pass in CI
- Channels for ownership transfer and coordination; mutexes for protecting shared state. Do not substitute one for the other
- Never embed `sync.Mutex`/`sync.RWMutex`; keep it as a named unexported field (`mu sync.Mutex`) next to the fields it guards
- Never copy a struct that contains a mutex; pass by pointer
- Never write to a map concurrently without synchronisation
- Every goroutine has a clear termination condition, normally `ctx.Done()`; a goroutine leak is a bug
- Wait with `sync.WaitGroup` or channels, never `time.Sleep`; time-dependent code takes a clock interface so tests use a fake clock
