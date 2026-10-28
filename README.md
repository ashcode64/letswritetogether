# Let's Write Together

One shared document. One word a minute. Everyone gets a say.

Let's Write Together is a turn-based crowd essay. The page is a single document for the current UTC day. Every minute, anyone online can submit one fragment and vote on the fragments submitted in that minute. When the minute ends, the winning fragment is appended. The next minute starts with an empty ballot. A full day is at most 1,440 fragments, one for each minute that actually elected one.

The screen is the document. A light red box after the last word opens the round. Past days that contain at least one winning word are listed as paper files and loaded only when opened.

## How a minute works

The server clock is Redis `TIME`. Nothing else is allowed to decide the round.

- `round_id = floor(unix_seconds / 60)`
- The first 30 seconds are open for submissions. After that the composer closes.
- Voting stays open for the whole minute, including the submission window.
- Each device may submit one fragment per minute and may change its vote until the minute ends.
- You can vote for any submitted fragment, including your own.
- Vote counts stay hidden until the minute ends.
- The fragment with the most votes is appended.
- If two or more fragments share that top count, the one submitted first wins. Submission order is the numeric word id assigned when the text was first accepted.
- If nobody submitted, or nobody voted, the minute is skipped. Nothing is appended and the document does not grow a blank slot.

The browser greys the composer when the submit window closes and shows "Voting only, submit a word in the first 30 seconds of the next minute." The server still enforces the window: a late submit is `409 SUBMISSION_CLOSED`, and a vote for a round that is no longer current is `409 VOTING_CLOSED`.

## The page

The document is a flat white US Letter sheet on a cream background. When the text fills a sheet, another sheet is added underneath. Spacing follows the essay: a normal word gets one space before it, and punctuation that binds to the previous word does not.

**How it works** sits under the title and expands in place.

**Past submissions** opens the last 31 days that actually contain a winning word. Dates are shown on file icons. A day's text is fetched the first time that file is opened and then kept in memory.

The round dialog has:

- Two clocks at the top: time left to submit a word, and time left to vote.
- A search field. Typing filters the in-memory list and is case-sensitive. Search refetches the ballot so a fragment accepted in that second can appear.
- The ballot, as selectable buttons that wrap to about four to six per line. The list is shuffled per device so an early fragment is not on top for everyone. Your own fragment is tagged "yours". The fragment you are voting for is highlighted with a star.
- A composer that accepts one fragment during the first 30 seconds.

## What counts as the same word

The ballot keeps the exact text. Two submissions share one vote button only when every character matches, including case, spaces, and punctuation.

`hello`, `heLLO`, `HELLO`, `hello!`, and `hello-` are five candidates. `h e l l o` is not `hello`. A second device that sends an identical string joins the existing candidate and has still used its one submission for the minute. The next minute starts empty, so a fragment that already won today can be submitted again.

A fragment is 1–48 characters after trimming only the leading and trailing whitespace. Internal spaces stay. Letters A–Z and essay punctuation are allowed anywhere:

`. , ! ? ; : ' " - — – … ( ) [ ] { } ‘ ’ “ ” & /`

Digits and other symbols (`@ # $ % *` and the rest) are rejected with `422 BAD_WORD`. Non-ASCII letters such as `café` are rejected.

## Moderation

The blocklist does not see the ballot text. It sees a letter core: casefolded, accents stripped, spaces and the punctuation above removed. `HELLO`, `hello!`, and `h e l l o` therefore hit a ban on `hello`. A fragment that is only punctuation has an empty letter core, skips the lists, and is allowed.

Matching is exact on that core. There is no substring match and no edit distance.

1. Latin-script lists from [LDNOOBW](https://github.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words), used under [CC BY 4.0](data/blocklist/LICENSE).
2. If the core is not on the English allowlist (`data/blocklist/allow.txt`, ~10k everyday words), an Indic phonetic skeleton is compared with lemmas in `data/blocklist/indic.txt` and long `exact/hi.txt` entries. Skeletons shorter than three consonants are ignored so words like `ice` do not collide with `choochi`. The allowlist covers longer collisions such as `classic`.
3. Exact Arabizi, Latin-transliterated Russian, and a short hand list for other romanizations.

A hit is `422 WORD_REJECTED` and is never published. A pass publishes the original text, not the letter core. No list blocks every language. Spellings that are not on a list pass until a lemma is added.

## Architecture

Seven Go 1.23 services. Public writes are REST. Live updates are Server-Sent Events. Redis holds the live minute. Postgres stores essays and nothing else on the hot path. Browser traffic and service-to-service REST go through **two Mendr data-plane edges** (public + private), each on its own control-plane tenant, so the public edge never syncs `/internal` routes. Redis pub/sub does not go through Mendr.

```mermaid
flowchart LR
  browser[Browser] --> pub[mendr-gateway-public]
  pub -->|"Host: sharedpage.app"| gateway[gateway]
  gateway -->|"Host: sharedpage-internal"| priv[mendr-gateway-private]
  orchestrator[orchestrator] -->|"Host: sharedpage-internal"| priv
  priv --> submission[submission]
  priv --> voting[voting]
  priv --> canvas[canvas]
  priv --> realtime[realtime]
  priv --> archive[archive]
  submission --> redis[(Redis)]
  voting --> redis
  orchestrator --> redis
  realtime --> redis
  canvas --> pg[(Postgres)]
  archive --> pg
```

| Service | Role |
| --- | --- |
| gateway | Mints the device cookie, rate-limits, serves the page, and forwards public REST to the **private** Mendr edge with `Host: sharedpage-internal`. |
| submission | Checks the charset and blocklist, stores one exact text per round, and publishes `word.approved`. |
| voting | Applies votes in one Redis Lua script. Tallies the minute when the orchestrator asks. Does not publish per-vote counts. |
| orchestrator | Holds the lease, closes each minute, appends a winner or records a skip, and asks archive to seal at UTC midnight. |
| canvas | Appends a winning fragment, idempotent on `(day, round)`. Serves today, one past day, and the list of days that have words. |
| archive | Seals a UTC day and deletes days older than 31. It is the only service that deletes rows. |
| realtime | Holds SSE sockets and fans out `word.approved`, `round.phase`, `round.winner`, and `round.skipped`. A slow client is dropped. |

Submission, voting, canvas, archive, and realtime do not call each other.

The orchestrator is the only writer of the phase key. It takes a Redis lease (`SET NX PX`) and renews it. If it loses the lease it does not close a round. A stored outcome makes a retry a no-op, so a skip cannot later become a blank slot and a winner cannot be appended twice. The unique key is `(day, round_number)`.

Public Host tables send traffic **only** to `sharedpage-gateway`. `/internal/*` exists only under Host `sharedpage-internal` on the **private** edge. See [docs/MENDR_INTEGRATION.md](docs/MENDR_INTEGRATION.md), [docs/MENDR_ONBOARDING_RUNBOOK.md](docs/MENDR_ONBOARDING_RUNBOOK.md), and [docs/adr/0002-two-mendr-edges.md](docs/adr/0002-two-mendr-edges.md).

## Public API

The browser calls these on the public site. `POST /v1/sessions` and `GET /v1/rounds/current` are answered by the gateway. Everything else is proxied through Mendr to the owning service.

| Method | Path | Body / result |
| --- | --- | --- |
| `GET` | `/v1/sessions/config` | Turnstile site key. No session. |
| `POST` | `/v1/sessions/challenge` | A short proof-of-work challenge. |
| `POST` | `/v1/sessions` | Device-key proof, proof-of-work, and a Turnstile token. Returns a 5-minute access token and sets a rotating HttpOnly refresh cookie. |
| `POST` | `/v1/sessions/refresh` | Device-key proof plus the refresh cookie. The old cookie cannot be used again. |
| `GET` | `/v1/rounds/current` | `round_id`, `phase`, `server_time`, time left to submit, time left to vote, and the shuffle seed. |
| `POST` | `/v1/rounds/{id}/words` | `{ "text", "seq" }`. One submission per device per minute. |
| `GET` | `/v1/rounds/{id}/words` | Approved fragments for this minute. No counts. |
| `PUT` | `/v1/rounds/{id}/vote` | `{ "word_id", "seq" }`. Last write wins for `(round, device)`. |
| `GET` | `/v1/events` | SSE: `word.approved`, `round.phase`, `round.winner`, `round.skipped`. |
| `GET` | `/v1/canvas/today` | Today's winning fragments, in election order. `Cache-Control: no-store`. |
| `GET` | `/v1/canvas/days` | Dates in the last 31 days that have at least one winner. |
| `GET` | `/v1/canvas/{day}` | One sealed or open day. A sealed day may be cached. |

Errors are `application/problem+json`. Codes include `SUBMISSION_CLOSED`, `VOTING_CLOSED`, `ALREADY_SUBMITTED`, `WORD_REJECTED`, `BAD_WORD`, and `STALE_SEQ`. A stale vote is HTTP 200 with `STALE_SEQ` and writes nothing. The client keeps a monotonic `seq` per round in `sessionStorage`. An older retry cannot overwrite a newer vote.

## Internal API

These routes require a 30-second Ed25519 JWT. Tally, append, and seal accept only a token signed by the orchestrator key. The gateway key cannot close a round. Internal paths are registered only on the **private** Mendr Host `sharedpage-internal`, not on the public edge.

| Method | Path | Effect |
| --- | --- | --- |
| `POST` | `/internal/v1/rounds/{id}/tally` | Winner, or a skip when the ballot was empty or received no votes. |
| `POST` | `/internal/v1/canvas/append` | Insert the winning fragment for that UTC day and round. |
| `POST` | `/internal/v1/days/{day}/seal` | Seal the day and drop days older than 31. |

## Storage

Redis holds the live minute: phase, lease, exact-text dedupe, word ids, per-device submission, per-device vote, tally, and the closed-round outcome. Round keys expire after 180 seconds. The closed-round marker is kept for 48 hours. Redis is capped at 256 MB with `volatile-lru`.

Postgres 16 stores `days` and `slots`. A slot exists only for a minute that elected a fragment. `word` is never null. Canvas reads Redis `TIME` when it decides what "today" means, so the essay day matches the orchestrator's UTC day.

## Run it locally

1. Copy `.env.mendr.example` → `.env.mendr` and set `MENDR_CONTROL_PLANE_URL` plus `GATEWAY_EDGE_API_KEY_PUBLIC` and `GATEWAY_EDGE_API_KEY_PRIVATE` (two CP tenants).
2. Deploy (starts both Mendr edges and registers public + private tenants).

```bash
# Linux / macOS / Git Bash
./mendr/scripts/deploy.sh

# Windows PowerShell
.\mendr\scripts\deploy.ps1
```

Or plain Compose — `mendr-register-public` and `mendr-register-private` run when `MENDR_AUTO_REGISTER=true`:

```bash
docker compose --env-file .env.mendr up --build -d
# re-register later:
docker compose --env-file .env.mendr run --rm mendr-register-public
docker compose --env-file .env.mendr run --rm mendr-register-private
```

Open `http://localhost/`.

Production overlay: cloudflared dials out to Cloudflare and targets `http://mendr-gateway-public:80`. Port 80 is not published. The private edge stays unpublished. The gateway trusts `CF-Connecting-IP`.

```bash
DEPLOY_PROD=1 ./mendr/scripts/deploy.sh
# or
docker compose -f docker-compose.yml -f docker-compose.prod.yml --env-file .env.mendr up --build -d
```


Compose defaults: Postgres user/password/db `colorbook`, `COOKIE_SECRET=dev-cookie-secret`, Cloudflare always-pass Turnstile test keys, and dev Ed25519 seeds. Change those before exposing a host. The Go image tag is `colorbook:local`. UI changes need `docker compose up --build -d gateway`.

Go 1.23 is the language version:

```bash
go test ./...
```

`cmd/load` is a small crowd against the public origin:

```bash
go run ./cmd/load -url http://localhost -painters 20 -viewers 20 -hold 3s
```

## Layout

```
cmd/            one main package per service, plus cmd/load
internal/       clock, words, filter, essay join, Redis scripts, and each service
web/            the page, embedded by the gateway
mendr/          OpenAPI, register scripts, optional manifest
docs/           Mendr integration + ADR
data/blocklist  vendored lists and data/blocklist/LICENSE
```


The module path is `colorbook`. The name on the page is Let's Write Together.

## Host

The intended host is one VM with 2 cores and 12 GB of RAM. Compose runs Mendr data plane, Redis, Postgres (`shared_buffers=256MB`), and the seven Go binaries. The Mendr edge must share that Docker network so upstream names like `gateway` resolve. Control plane may be remote. Raise the open-file limit before expecting thousands of SSE sockets.
