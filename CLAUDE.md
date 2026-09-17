# This fork, and who eats it

Fork of [`aldinokemal/go-whatsapp-web-multidevice`](https://github.com/aldinokemal/go-whatsapp-web-multidevice)
(`penard-monkey/go-whatsapp-web-multidevice`). `AGENTS.md` maps the upstream
project — structure, where to add an endpoint, the migration list. This file is
the part `AGENTS.md` cannot know: **what consumes this binary downstream, and
what it will break.**

The only consumer is **`~/workspace/saywhat`** — WhatsApp voice notes
transcribed on arrival, a Mac app, a CLI, and an MCP server. It runs this
binary as `net.casadelvalle.saywhat.gowa`, a launchd agent on 127.0.0.1:3210,
holding **the live paired WhatsApp session for a real phone number in daily
use**. That is the whole reason this fork exists: we needed to change the
server, and a pinned upstream release cannot be changed.

## How saywhat runs it

Rendered from `saywhat/launchd/net.casadelvalle.saywhat.gowa.plist.tmpl`, with
`__HOME__` = `~/.local/share/saywhat`:

```
<home>/gowa/bin/gowa rest
  --host=127.0.0.1 --port=3210
  --os=saywhat
  --webhook=http://127.0.0.1:3220/hook
  --webhook-secret=<from <home>/webhook.secret>
  --webhook-events=message,message.ack,message.revoked,message.deleted,message.edited
  --auto-download-media=true
  --presence-on-connect=unavailable
  --presence-pulse-enabled=false
```

`WorkingDirectory` is `<home>/gowa`, so `storages/` and `statics/media/` land
there. `KeepAlive` is on: the job comes back by itself, which also means a
binary swapped under it takes effect at the next crash or restart, not when you
swap it.

## The contract saywhat depends on

Change any of these and saywhat breaks quietly — usually as "no new
transcripts", which nobody notices for a day.

| What | Where it lands | Notes |
| --- | --- | --- |
| Webhook POST to `/hook`, signed `X-Hub-Signature-256` | `saywhat-core/src/webhook.rs` | An unsigned or wrongly signed body is rejected 401. The secret is shared by the plist, not negotiated. |
| Event names `message`, `message.ack`, `message.revoked`, `message.deleted`, `message.edited` | `saywhat-core/src/daemon.rs` | Renaming one silently stops that behaviour. `message.deleted` and `message.revoked` both mean "tombstone this". |
| Payload field names (`chat_id`, `from`, message id, media fields) | same | saywhat reads shapes, not a typed upstream schema. |
| Auto-downloaded media under `<home>/gowa/statics/media/...` | `saywhat-core/src/media.rs` | saywhat stores the **relative** path and joins it to `<home>/gowa`. Moving or renaming that tree orphans every indexed attachment. |
| `GET /chat/{jid}/messages?limit&offset` | `saywhat-cli/src/mcp.rs`, `saywhat-core/src/gowa.rs` | **`limit` over 100 is a `VALIDATION_ERROR`, not a short page.** saywhat clamps to 100 and pages; if that cap ever changes, `gowa::MAX_PAGE` follows it. |
| `/mcp` (streamable HTTP), needs `X-Device-Id` | Claude's `gowa` MCP server | Sending, reacting and raw history are this server's job. saywhat's own MCP never duplicates them. |
| Message id format | everywhere | saywhat validates ids against `[A-Za-z0-9_-]{1,64}` before they reach a URL or `open`. An id with other characters would be refused as malformed. |

## Building, and getting it onto the machine

saywhat owns the install; do not hand-copy a binary into `<home>/gowa/bin`.

```sh
cd ~/workspace/saywhat && services/gowa/build.sh
```

That builds this checkout (`GOWA_SRC` overrides the path), refuses a dirty tree
unless you pass `--allow-dirty`, stamps `config.AppVersion` with the commit,
installs to `<home>/gowa/bin/gowa`, and keeps the previous binary as
`gowa.previous`. `services/gowa/SOURCE` records this fork's URL and the ref
saywhat expects.

`services/gowa/fetch.sh` — the stock upstream release — still exists as the
fallback for a machine with no checkout. It now refuses to overwrite a fork
build unless forced, because it silently reverting us to stock is exactly the
failure this fork is here to prevent.

## Restarting is not free

**Never `launchctl bootout`/`kickstart` the gowa job to "just try it".**
Bouncing it drops the live WhatsApp session, and a session that fails to come
back means re-pairing a real phone by QR. saywhat's own installer goes out of
its way to leave this job alone (`make install-services` touches only the
daemon). A new build is picked up on the next restart — when you actually mean
it:

```sh
launchctl kickstart -k gui/$(id -u)/net.casadelvalle.saywhat.gowa
tail -f ~/.local/share/saywhat/logs/gowa.log     # watch it reconnect
~/.local/bin/saywhat doctor                      # every line green
```

Roll back by putting `gowa.previous` back and kickstarting again.

## Working here

- **Our patches live in `PATCHES.md`.** Every change on top of upstream is a
  row there with a sentinel string, and `scripts/check-patches.sh` fails if one
  vanishes. saywhat's `build.sh` runs it and refuses to build without it, so a
  patch lost in a sync cannot reach a binary.
- **Nothing about this is automatic in git.** A merge or rebase conflicts only
  when upstream touched the *same lines*; a refactor around our code merges
  clean and silently stops running it, and a reset to upstream drops everything
  without a word. The guard is the reminder — do not rely on remembering.
- **Sync with `scripts/sync-upstream.sh`** (adds the `upstream` remote if it is
  missing, rebases, then runs the guard and the full suite). Rebase, never
  merge, so the patches stay a readable series.
- **Mark every patched site** with a `SAYWHAT-PATCH: <id>` comment, and cover
  it with a Go test. A conflict inside one of those blocks during a rebase is
  the system working.
- **Keep changes as commits, never as a dirty tree.** A binary built from
  uncommitted edits cannot be rebuilt: `go version -m` reports only
  `vcs.modified=true`. That is how the `template-summary` patch was nearly
  lost — recovered on 2026-09-17 by diffing the running binary's symbol table
  against a clean build of the same revision.
- Go 1.26 per `src/go.mod`; the module root is `src/`, so run Go commands
  there, not at the repo root.
- Run `cd src && go build ./... && go test ./...` before proposing a build.
