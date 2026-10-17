# Agent and Tink Server Retry

Status: implemented

## Problem

Retries happened in two places. The Agent backed off between `GetAction` calls, and the
Tink Server wrapped every `GetAction` and `ReportActionStatus` in its own retry loop
(once a second, for up to a minute) for any error not marked permanent.

The most common answer, "no Workflow for this Agent", was one of those errors. So each
idle Agent's request sat on the server for about a minute, re-reading the cache once a
second, and the Agent's backoff covered only the few seconds between requests:

- Server load was N idle Agents × 1 lookup per second, and Agent settings couldn't change it.
- The two layers multiplied: every Agent attempt became up to 60 server attempts.
  Enrollment nested a second retry loop inside the first.
- The server's fixed 1s retry had no jitter, which undid the Agent's jitter.
- The retry settings were stored on the handler the first time a request ran, so
  concurrent requests raced on them.
- A graceful shutdown waited on requests that were being held for up to a minute.

## Principle

**Retry in one place, the Agent.** The server makes one attempt per request and
returns a status code that tells the Agent what kind of result it got.

The server-side retry existed to ride out a stale cache: a status write from a cached
Workflow can conflict, and a just-created Workflow isn't in the cache yet. The Agent
already retries in every one of those cases, so the server-side loop only added load
and hid it. Each idle poll is now an indexed, in-memory cache lookup, so server load is
about `agents / poll interval`, and the Agent controls it.

## Contract

The status code tells the Agent what to do next.

| Server result | Code | Agent |
|---|---|---|
| Action served | `OK` | Run it, no delay |
| No Workflow, no Action, preparing, terminal, enrollment disabled or pending | `NotFound`, `FailedPrecondition` | Idle: poll again later |
| Write conflict (the Workflow changed after it was read) | `Aborted` | Retry soon |
| Backend or transport failure | `Internal`, `Unavailable`, … | Exponential backoff |
| Report rejected, including a deleted Workflow | `NotFound`, `InvalidArgument`, `FailedPrecondition` | Stop retrying the report and go back to polling |

Idle polling (`GetAction`) reads the server's cache, so it's cheap. A stale cache can't do harm
there: the worst case is an idle answer, or a write conflict that is retried. Action status
reports (`ReportActionStatus`) read the Workflow from the API server instead. Reports are
infrequent, two per Action, and a stale cache could otherwise reject a valid final report,
which would make the Agent run the Action again.

## Agent retry policy

| Situation | Delay |
|---|---|
| Idle or report rejected | Random in `[poll/2, 3·poll/2)`, so Agents don't fall into step |
| Conflict | Random in `[0, min(500ms, max))` |
| Error | Full jitter: random in `[0, min(max, 1s·2^n))`, where `n` is the number of consecutive errors; any other result resets `n` |

The first poll happens immediately; the jitter on the first idle answer spreads out Agents
that start together. The final Action report is retried until it is accepted or rejected as
permanent. Every gRPC call has a 30s deadline, so a dead connection can't stall the Agent.
The gRPC transport classifies errors; the Agent's loop works the same for every transport.

## Upgrading

Upgrade the Tink Server before the Agents, or keep their versions matched. The chart already
does this unless `deployment.agentImageTag` is pinned. A new Agent against an old server hits
its 30s deadline on every idle poll, because the old server holds the request for up to a
minute; it treats that as an error and backs off. An old Agent against a new server works,
but logs each idle poll as an error.

## Configuration

| Flag | Default | Meaning |
|---|---|---|
| `--poll-interval` | `5s` | Average delay between polls while idle |
| `--backoff-max-interval` | `30s` | Cap on the error and conflict delays (was effectively the idle poll cap, default `5s`) |
| `--grpc-retry-interval` | — | Removed. It was never used |

## Not in scope

- **Long-poll.** The server holds `GetAction` until a Workflow informer event, so Actions
  are dispatched immediately and idle Agents cost nothing. Add it only if pickup latency
  (about half the poll interval on average) matters.
- **Server-suggested delay** (the `RetryInfo` error detail). This would let the server
  shed load centrally. It isn't needed while Agents control the poll rate.
