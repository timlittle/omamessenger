## Go Style

Baseline: the [Google Go Style Guide](https://google.github.io/styleguide/go/) (guide, decisions, best practices). The rules below add to it or tighten it.

- Functions: aim for ~30 lines; hard limit 50. Extract a helper if longer
- Max 3 levels of indentation. Extract a function if deeper
- Happy path left-aligned: handle errors first, return early, keep the success path unindented
- Omit `else` after `return`/`break`/`continue`
- Group a function body into short paragraphs separated by one blank line: validate inputs, do the work, return. No blank line after `{` or before `}`
- Every package has a package doc comment (`// Package store persists …`) in one file
- Every function, method, type, constant and variable has a doc comment, exported or not, starting with its name: `// openStore creates the database file and applies migrations.`
- Comments explain *why* in plain English. Do not restate the code
- No plan or task IDs (`B06`, `C3`, `F15`) and no invented jargon in code, comments, test names or commit messages. The code must read on its own
- Acronyms all-caps in identifiers: `URL`, `HTTP`, `ID`, `RPC`. Never `Url`, `Id`
- Receiver names: 1–2 letters, consistent across all methods of a type; never `this` or `self`
- Package names: short, lowercase, single word; no `util`, `common`, `helpers`, `base`
- Formatting: `gofumpt` (enforced by `make lint`)
- Remove dead code, unused parameters and leftover workarounds in the same change that makes them obsolete
- After writing Go code, run `make check` before reporting done
