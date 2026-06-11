# Clean discovery adds per-user system caches and deliberately excludes browser caches and the Recycle Bin

Clean's review-only skipped-by-default discovery generalizes from a single user-temp scan to multiple opportunity categories, each with its own observation rule. The JSON `opportunities` array gains a `category` field. The v1 categories beyond user temp are per-user, no-admin, no-external-tool, regenerating caches: crash dumps (`%LOCALAPPDATA%\CrashDumps`), Windows Error Reporting (`WER`), the Explorer thumbnail cache, `INetCache`, and GPU shader caches (`D3DSCache`, NVIDIA `DXCache`). Each is a fixed known root observed whole as one opportunity, gated on existence rather than idle age (these caches regenerate, so age conveys no safety signal), reusing the existing 100k-descendant inspection ceiling and reparse-point rejection.

Two obvious, high-value targets are deliberately left out, and a future reader will ask why:

- **Browser caches (Chrome/Edge/Firefox)** are the largest single prize and were still excluded from v1. `CONTEXT.md` lists browsers under *Running application skip*: observing and recommending a browser cache while the browser is running is exactly the unsafe case the glossary calls out. Doing it correctly requires the process detection that was deliberately deferred (see ADR 0004's sibling decision to fold running-app detection into a static caveat instead of enumerating processes). Browser caches are therefore blocked on building real running-application detection first.
- **The Recycle Bin** is excluded on principle, not effort. Foal's entire model is *move to the Recycle Bin*; surfacing "you could empty the Recycle Bin" as a cleanup opportunity undercuts the safety net Foal itself depends on.

System-wide caches that need administrator rights (`SoftwareDistribution`, Delivery Optimization) are reported as permission-boundary skips, never opportunities, consistent with Foal's no-automatic-elevation stance.

## Considered options

- **Include browser caches in v1 for the byte payoff** — rejected: cannot be observed safely without running-application detection, which is not yet built; shipping it now would contradict the glossary's running-app concern.
- **Include the Recycle Bin as an opportunity** — rejected: philosophically incoherent for a Recycle-Bin-first tool and risks users emptying the recovery path Foal relies on.
- **Keep a single user-temp scan and add no categories** — rejected: leaves real, low-disagreement reclaimable space (crash dumps, WER, shader caches) invisible to users.

## Consequences

The `category` field is a JSON contract addition that is awkward to remove later. A future contributor will be tempted to "just add browser cache and the Recycle Bin" because they are the biggest, most obvious targets — that temptation is the reason this ADR exists. Browser caches stay out until running-application detection lands; the Recycle Bin stays out permanently. Each new category must remain per-user, admin-free, free of an owning external tool (or it belongs in Review suggestions), and observable within the existing inspection ceiling.
