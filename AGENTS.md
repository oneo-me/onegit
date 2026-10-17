# Onegit

Onegit is a worktree-centered macOS app for browsing many Git repositories. Read [ARCHITECTURE.md](ARCHITECTURE.md) in full before coding, then the relevant subjects. Go commands run in `source/`.

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

- Onevra is consumed as the published module version in `source/go.mod`; never copy its code or add a `replace` directive without the user's agreement. Discuss any Onevra change with the user first, then follow that repository's own instructions.
- Use the MyGo CLI through `go tool mygo`; never install it globally. Build output goes to the root `dist/`, and Neorio tasks live in `.neorio/config.yaml`.
- Commit only when authorized. Never push, tag or publish without explicit authorization.
- Commit subjects are Conventional Commits naming the outcome in the product's words, such as `feat: delete refs from their menu`, with no body unless breaking.
- Never launch the app, `go tool mygo dev` or anything else that opens windows, native dialogs or the Dock; the user reviews UI natively. Report what you could not verify.

## Verification

- While iterating, run `gofmt -l .` and `GOWORK=off go test` only for the affected packages, narrowed with `-run` where it helps.
- Before handing off, run the commands in [README Development](README.md#development) once, skipping plain `go test ./...` because the race run covers it, and `git diff --check` from the repository root. Do not rerun them when nothing changed since they passed.
- Test UI headlessly with `ui.NewTester`, and inject native dialogs and other platform calls so tests never open them.
- Add tests for new behavior and fixed bugs, extending an existing test where one fits; not for styling, copy, layout tweaks or behavior-preserving refactors.

## Documentation

- Update a document only when a fact its reader needs changes. README is onboarding, ARCHITECTURE.md is boundaries and rationale, CHANGELOG.md is user-facing release notes.
- Keep budgets: AGENTS.md 4 KiB, each README 8 KiB, ARCHITECTURE.md 12 KiB, each subject 16 KiB.
- CHANGELOG.md records what a user notices or must do: one short sentence per outcome, in the product's words. Omit refactors, tests, styling, copy tweaks, fixes to unreleased features and internal work; merge related entries instead of appending task histories.
- When you edit a document, also trim stale, duplicated or overly detailed text in it.
- Keep `README.md` and `README.zh-CN.md` equivalent, with reciprocal language links; update both in the same change.
- Keep paragraphs to about four short sentences and list items to two lines; use lists and tables for steps and mappings.
- Document only commands that exist and work.
- `CLAUDE.md` is a relative symlink to `AGENTS.md`; edit `AGENTS.md`.
- Keep documentation on the code branch and reconcile it during merges.
