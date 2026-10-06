## Interface Design

- Keep interfaces small: prefer 1–2 methods; 3+ requires a justification in the doc comment
- Name single-method interfaces with the `-er` suffix: `Sender`, `Notifier`, `MessageStorer`
- Define interfaces at the consumer, not beside the implementation; the consumer owns the contract
- Accept interfaces, return concrete types
- Verify satisfaction at compile time where the pairing matters: `var _ Sender = (*Manager)(nil)`
- An interface for testability is valid with one production implementation; the test fake is the second
- Do not mirror a struct's whole API as an interface; a consumer that needs many methods is doing too much
- Do not use `any` where a concrete type or constrained generic will do
- The connector interfaces (`Connector`, `Sink`) are fixed: a new capability is a new optional interface that callers type-assert, so existing connectors keep compiling
