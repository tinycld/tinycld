## What we know

Boards with **200+ cards** take several seconds to first paint.
Profile `useActiveBoard` and the initial render before changing anything.

### Suspects

- The six live queries that feed the board tree
- Structural sharing missing a field, so every column re-renders
- Label and assignee lookups resolving per card

| Board size | First paint |
| ---------- | ----------- |
| 50 cards   | fine        |
| 200 cards  | \~2s        |
| 500 cards  | \~6s        |

> Measure first. The last two "obvious" fixes here made it slower.

See [the board query notes](https://example.com/notes).
