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
| `read-self-receipt` | `POST /message/{id}/read-self`: a read receipt only your own devices see, whatever the account's read-receipt privacy says. Upstream's `/read` sends `read`, which whatsmeow downgrades to `read-self` only while privacy is `none` (judged on a cached setting); with receipts on it is a blue tick to the sender. saywhat calls only this route. The receipt type is hard-coded, never taken from the request — `played` is not reachable through it. | `MarkAsReadSelf` | `src/usecase/message.go`, `src/domains/message/interfaces.go`, `src/ui/rest/message.go` | `TestMarkAsReadSelfSendsOnlyReadSelfInADirectChat`, `TestMarkAsReadSelfResolvesTheGroupSenderLikeMarkAsRead`, `TestMarkAsReadSelfRefusesAnUnknownGroupMessage`, `TestMarkAsReadSelfRouteDelegatesToMessageService` |
| `webhook-content-length` | Preserve Content-Length and rewind the full request body for retries. Existing framing fix, now registered in the patch guard. | `GetBody` | `src/infrastructure/whatsapp/webhook.go` | `TestSubmitWebhookSendsContentLength`, `TestSubmitWebhookRetryKeepsFullBody` |
| `webhook-client-pool` | Share bounded webhook connection pools by effective TLS policy and drain/close responses before retrying, preventing a stranded idle socket per delivery. | `webhookClients` | `src/infrastructure/whatsapp/webhook.go` | `TestSubmitWebhookReusesConnections`, `TestWebhookClientsKeepTLSPoliciesSeparate` |

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
