## Workflow

- Plans live in `docs/plan.md` as self-contained briefs: each says what to build, the files it touches, the rules that apply and how to verify it. Plan IDs stay in the plan; they never appear in code, comments, test names or commit messages
- Lasting design choices and their reasons go in `docs/decisions.md`. Read it before reversing one
- Keep each change small and focused on one brief. Delete code a change makes obsolete in the same change
- Before reporting done: run `make check` and paste the result. If something fails or was skipped, say so
- Update the README when setup, authentication, installation or shortcuts change, and `docs/api.md` when the helper API changes
- Commit messages: imperative, plain English, say what changed and why. Commit only when asked; never push unless asked
- Subagents start from a stale worktree base: run `git merge --ff-only main` first
