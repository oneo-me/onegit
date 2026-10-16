# Onegit

Onegit is a worktree-centered macOS app for browsing many Git repositories. Read [ARCHITECTURE.md](ARCHITECTURE.md) fully before coding, then the relevant subjects.

## Principles

- Keep the scope to everyday repository browsing and worktree switching. Do not grow Onegit into a full Git client without a request.
- The original requirement "no user operation" is still unclear. It may mean no Git writes, no manual refresh or no accounts; do not turn any reading into a constraint.
- Live status refresh and broader Git operations are not designed yet. Ask before choosing their behavior.
- Separate confirmed goals, proposals, open questions and implemented behavior. Never claim a feature works without code and verification.
- Check Onevra's current API and requirements before using it. Do not copy upstream example commands as project commands.

## Working with the user

- Keep app code and the Go module in `source/`, and run Go commands from there.
- Use the MyGo CLI through `go tool mygo`. Never install it globally.
- Build output goes to the root `dist/`. Neorio tasks live in `.neorio/config.yaml`.
- Commit, push or tag only when asked.

## Verification

Commands are in [README Development](README.md#development). Run tests and vet after code changes, and launch the app for UI changes. Native dialogs and packaging need manual checks.

## Documentation

- Update a document only when a fact its reader needs changes.
- Keep each document within its budget and written for its reader.
- Record completed user-visible outcomes as short `Unreleased` bullets, merging related ones.
- Repair documents you touch instead of only appending to them.
- Keep `README.md` and `README.zh-CN.md` equivalent; everything else, including code comments, is English.
- Keep documentation on the code branch and reconcile it during merges.
- Document only commands that exist and work.
