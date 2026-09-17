# Patches this fork carries

Every change we make on top of upstream is listed here, with a **sentinel** —
a string that must exist in the tree for the patch to be present.
`scripts/check-patches.sh` greps for each one and fails if any is missing;
saywhat's `services/gowa/build.sh` runs it before it will build a binary.

This exists because nothing in git is automatic about a fork's patches. A
merge or rebase from upstream only conflicts when upstream touched the *same
lines*; a refactor around our code merges cleanly and silently stops running
it, and a reset to upstream drops it without a word. That has already happened
once here — see the "template-summary" note below.

**Adding a patch:** commit it with a `SAYWHAT-PATCH: <id>` comment at every
site it touches, add a row here with a sentinel that would disappear if the
patch did, and cover it with a Go test. **Removing one:** delete the row too,
or the guard fails for everybody.

| id | What | Sentinel | Files | Test |
| --- | --- | --- | --- | --- |
| `template-summary` | Render a business template message (header / body / footer / buttons) as text. Upstream has no template handling at all, so these arrive with empty content and read downstream as a message that says nothing. | `FormatTemplateSummary` | `src/pkg/utils/whatsapp.go` | `TestFormatTemplateSummary`, `TestExtractMessageTextFromProtoRendersTemplate` |

## Syncing from upstream

`origin` is our fork; upstream is a separate remote you may have to add:

```sh
git remote add upstream https://github.com/aldinokemal/go-whatsapp-web-multidevice.git
scripts/sync-upstream.sh          # fetch, rebase this branch, re-run the guard
```

Rebase rather than merge, so our patches stay a readable series on top of
upstream. When a rebase conflicts inside a `SAYWHAT-PATCH` block, that is the
system working — resolve it by hand, then re-run `go test ./...` in `src/`.

If the guard fails after a sync, the patch is gone: recover it with
`git log --all -S<sentinel>` and re-apply, rather than shipping without it.

## History

- **2026-09-16** — `template-summary` was built into the running binary from an
  uncommitted working tree and never committed. `main` matched upstream, so the
  live server could not be rebuilt from source. Recovered on 2026-09-17 by
  diffing the binary's symbol table against a clean build of the same revision
  (`go tool nm`, which showed exactly `FormatTemplateSummary` and
  `formatHydratedTemplateButton` as the only additions) and rewriting it with
  tests. The guard in this file exists so that cannot repeat quietly.
