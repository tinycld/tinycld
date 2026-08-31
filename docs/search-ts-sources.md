# TS search sources (design)

Status: design only, and **the motivating premise turned out to be false** — read
"Is this needed?" first. Nothing here is implemented.

## Is this needed?

The plan justified TS search sources with "hosted tenants run no feature Go, so a
hosted org can only search core's sources." **That is not true.** A tenant on the
hosting router runs the *same* generated registrar the self-hosted composition
does — `tenantmain.Options.RegisterExtras` is documented as "the tenant's feature
Go … the SAME generated registrar the host composition uses (single-Register
contract): a per-org build links exactly its package set." So a hosted org whose
package set includes mail gets mail's Go search source, exactly like a
self-hosted workspace.

That removes the whole reason this was queued. **Recommendation: don't build it**
unless one of these becomes real:

- **A package ships without Go.** A pure-TS package (settings-only, or one whose
  data lives entirely in collections) has no Go module to register a source from,
  so it cannot be searchable today at any deployment. This is the one genuine
  gap, and it is narrow: such a package's rows are plain collection reads, which
  is precisely the case a declarative config (below) covers without running any
  tenant code.
- **Third-party packages must be searchable without a rebuild.** The per-org
  build is the gate today; if that ever becomes "drop in a package at runtime",
  Go registration stops being available and this design becomes load-bearing.

Until then the Go registry covers every shipped package, and adding a second
registration path would mean two ways to be searchable and two threat models,
for no capability anyone currently lacks.

## If it is built: mechanism

No new machinery is required — `core/server/davhooks` already solves Go→TS
callout for the DAV protocols. It registers a JS global taking handler functions;
`jsvm.Compiler` turns each into a `jsvm.Callable`; Go invokes it and reads the
exported value back. Search would install one binding (`searchSource`) and wrap
the returned `Callable` in a `search.Source`. Everything downstream — merge,
score, group, per-source timeout, panic isolation — is untouched, because a
`Source` is just a function.

Constraints inherited from that seam, both documented there:

- A handler is compiled **once at hook-load time** and **closes over nothing**;
  anything it needs must be inline. Registration must run on the loader VM
  (`OnLoaderInit`), not `OnInit` — the latter fires per pooled VM and would
  register the same source once per executor.
- Rows cross as plain values, so `Fields` must stay string/number/bool, as it
  already must for `--json`.

## The two questions the plan flagged — both resolved

**1. Concurrency under the pooled jsvm.** Not a problem. `jsvm.Callable` is
documented safe for concurrent use, and `vmsPool.run` (`third_party/pocketbase/
plugins/jsvm/pool.go`) confirms why: each call borrows a free executor, and when
every pooled VM is busy it **builds a one-off VM rather than blocking**. The
aggregator's existing parallel fan-out therefore needs no change and cannot
deadlock against itself. Worth citing rather than assuming, because this is the
*fork's* behaviour — the stock jsvm plugin has no Callable at all.
`HookPoint.Call` also invokes handlers outside its mutex (it copies the slice
under `RLock` first), so it adds no serialization.

**2. Untrusted-tenant threat model.** The load-bearing rule:

> **Scoping stays in Go. A TS source is never handed a user id and trusted to
> filter honestly.**

A source that receives `userID` and returns "that user's rows" is a cross-tenant
leak one bug — or one malicious package — away. The safe shape is that TS never
runs the authoritative query: it declares an `fts.Config`-shaped **description**
of what to search and Go executes it, the way `fts.MemberScope` already expresses
scoping declaratively. That keeps tenant code out of the data path entirely and
reuses machinery that exists. It also happens to be exactly what the one real gap
above (a Go-less package) needs, which is a good sign it is the right shape.

Denial-of-service needs no new work: `ExecTimeout` bounds each handler
(`pool.go`'s `budget`), and the aggregator's `sourceTimeout` plus per-source
panic recovery degrade a slow or throwing source to a `Partial` entry instead of
failing the request — the same isolation a misbehaving Go source gets.

## Open question, deliberately not answered here

Whether a TS source may run **arbitrary SQL** or only a declared FTS config. The
recommendation above assumes the latter, which is more restrictive than what
package Go can do. That asymmetry is probably right — a hosted tenant is not a
trusted operator — but it is a product decision about what third-party packages
may do, so it should be settled before implementation rather than inside it.
