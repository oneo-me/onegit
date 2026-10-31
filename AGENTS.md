# Onegit

Onegit is a worktree-centered macOS app for browsing many Git repositories. Read [ARCHITECTURE.md](ARCHITECTURE.md) in full before coding, then the relevant subjects. Go commands run in `source_mygo/`.

## Principles

- Keep the scope to everyday repository browsing and worktree switching. Do not grow Onegit into a full Git client without a request.
- The original requirement "no user operation" is still unclear. It may mean no Git writes, no manual refresh or no accounts; do not turn any reading into a constraint.
- Live status refresh and broader Git operations are not designed yet. Ask before choosing their behavior.
- Separate confirmed goals, proposals, open questions and implemented behavior. Never claim a feature works without code and verification.
- Onevra is the shared component library. Prefer its components and shells; add reusable UI capabilities there, not as wrappers or workarounds in Onegit.
- Use MyGo primitives directly, and read version-matched MyGo and Onevra signatures before using them. Frame builders never perform I/O or start processes.
- Never delete user work, reset branches or switch a worktree's branch without an explicit user action.
- Tests use temporary storage and repositories, never the real `~/.onegit/` directory.
- Write code comments and all documentation in English. Only `README.zh-CN.md` is Chinese; UI strings and test fixtures are product content.

## Working with the user

- Never create sub-Agents or Git worktrees; work directly in the current checkout.
- Onevra is consumed as the published module version in `source_mygo/go.mod`; never copy its code or add a `replace` directive without the user's agreement. Discuss any Onevra change with the user first, then follow that repository's own instructions.
- Use the MyGo CLI through `go tool mygo`; never install it globally.
- Commit only when authorized. Never push, tag or publish without explicit authorization.
- Commit subjects are Conventional Commits naming the outcome in the product's words, such as `feat: delete refs from their menu`, with no body unless breaking.
- Never launch the app, `go tool mygo dev` or anything else that opens windows, native dialogs or the Dock; the user reviews UI natively. Report what you could not verify.

## Verification

- While iterating, run `gofmt -l .` and `GOWORK=off go test` only for the affected packages, narrowed with `-run` where it helps.
- Before handing off, run the commands in [README Development](README.md#development) once, skipping plain `go test ./...` because the race run covers it, and `git diff --check` from the repository root. Do not rerun them when nothing changed since they passed.
- Test UI headlessly with `ui.NewTester`, and inject native dialogs and other platform calls so tests never open them.
- Add tests for new behavior and fixed bugs, extending an existing test where one fits; not for styling, copy, layout tweaks or behavior-preserving refactors.

## Documentation

- Update a document only when a fact its reader needs changes. README is onboarding, ARCHITECTURE.md is boundaries, CHANGELOG.md is release notes.
- Avoid secondary labels and descriptions that restate a heading or value; let the layout explain.
- Budgets: AGENTS.md 4 KiB, READMEs 8 KiB, ARCHITECTURE 12 KiB, subjects 16 KiB.
- CHANGELOG.md records what a user notices or must do: one short clause per outcome, in the product's words, with references such as `(#12 by @alice)`.
- Order as `Breaking:`, Add, other, Fix; merge related entries instead of task histories. Omit refactors, tests, styling, copy tweaks, unreleased fixes and internal work.
- When you edit a document, trim stale or duplicated text.
- Keep READMEs equivalent with reciprocal language links; update both together.
- Keep paragraphs to four sentences and list items to two lines; use lists and tables.
- Document only working commands.
- `CLAUDE.md` is a relative symlink to `AGENTS.md`; edit `AGENTS.md`.
- Keep documentation on the code branch and reconcile it during merges.
