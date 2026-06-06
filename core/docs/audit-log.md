# Audit Log

Server-side Go hooks automatically record create, update, and delete events for auditable collections. Logs are written to the `audit_logs` PocketBase collection and visible to org admins at **Settings > Audit Log**.

## What gets logged

Every API-driven create, update, or delete on the collections below produces an audit entry. System-initiated changes (e.g. Go hooks that bypass the API) are not captured.

### Core

| Collection | Label shown | Notes |
|---|---|---|
| `orgs` | org name | Org-level settings changes |
| `user_org` | role | Member added/removed/role changed |
| `labels` | label name | |
| `label_assignments` | — | Label applied/removed from a record |
| `settings` | `app:key` | Org settings key-value changes |
| `org_pkg_access` | — | Per-member package access overrides |
| `org_pkg_enabled` | — | Package enabled/disabled for an org |

### Contacts

| Collection | Label shown |
|---|---|
| `contacts` | contact name |

### Calendar

| Collection | Label shown |
|---|---|
| `calendar_calendars` | calendar name |
| `calendar_events` | event title |
| `calendar_members` | — |

### Drive

| Collection | Label shown |
|---|---|
| `drive_items` | file/folder name |
| `drive_item_state` | — |
| `drive_shares` | — |

### Mail

| Collection | Label shown |
|---|---|
| `mail_domains` | domain |
| `mail_mailboxes` | email address |
| `mail_messages` | subject |
| `mail_thread_state` | folder |
| `mail_mailbox_members` | — |

## What each entry contains

| Field | Description |
|---|---|
| `org` | The organization the action belongs to |
| `actor` | The user who performed the action (empty for system actions) |
| `action` | `created`, `updated`, or `deleted` |
| `resource_type` | Collection name (e.g. `contacts`, `drive_items`) |
| `resource_id` | The ID of the affected record |
| `resource_label` | Human-readable name (contact name, file name, email subject, etc.) |
| `changes` | For updates: field-level diffs as `{"field": {"before": old, "after": new}}` |
| `snapshot` | For deletes: full record data at time of deletion |
| `ip_address` | Client IP from the request |
| `user_agent` | Browser/client user agent string |
| `metadata` | Extra context (e.g. `{"source": "system"}` for non-request actions) |

## Redacted fields

Sensitive fields are never included in diffs or snapshots. If a redacted field changes, the diff entry shows `{"redacted": true}` instead of before/after values.

- `password`
- `passwordConfirm`
- `tokenKey`
- `keys`

## API rules

- **List/View**: any authenticated org member (`org.user_org_via_org.user ?= @request.auth.id`)
- **Create**: server-only (null) — entries are written by Go hooks, not client code
- **Update**: null — audit logs are append-only
- **Delete**: null — audit logs cannot be deleted through the API

## Architecture

The audit system is a shared Go package at `server/audit/` (`tinycld.org/core/audit`). Each package owns its own audit registration — core collections are registered in `server/audit.go`, and each package registers its collections in its own `Register()` function (e.g. `packages/contacts/server/register.go`).

This means packages don't need to modify any central file to add audit logging.

## Adding a new collection to the audit log

In your package's `Register()` function, call `audit.RegisterCollection`:

```go
import "tinycld.org/core/audit"

func Register(app *pocketbase.PocketBase) {
    // Direct org field — default resolver handles it
    audit.RegisterCollection(app, "my_collection", &audit.CollectionConfig{
        ExtractLabel: audit.LabelFromField("name"),
    })

    // Custom org resolution via relation chain
    audit.RegisterCollection(app, "my_child_collection", &audit.CollectionConfig{
        ResolveOrg: func(a core.App, record *core.Record) string {
            parentID := record.GetString("parent")
            return audit.ResolveViaRelation(a, "my_collection", parentID, "org")
        },
        ExtractLabel: audit.LabelFromField("title"),
    })
}
```

### API

- **`audit.RegisterCollection(app, name, config)`** — registers create/update/delete hooks for one collection
- **`audit.RegisterCollections(app, names, config)`** — registers the same config for multiple collections
- **`audit.CollectionConfig`** — optional config with:
  - `ResolveOrg` — `func(app core.App, record *core.Record) string` to find the org ID. If nil, the default resolver checks `org`, `owner→user_org`, and `user_org` fields.
  - `ExtractLabel` — `func(record *core.Record) string` to extract a display label. If nil, tries `name`, `title`, `label`, `address`.
- **`audit.LabelFromField("name")`** — returns a LabelExtractor for a single field
- **`audit.LabelFromFields("app", "key")`** — returns a LabelExtractor that joins fields with `:`
- **`audit.ResolveViaRelation(app, collection, id, field)`** — loads a related record and reads a field (useful for building org resolver chains)

### Steps

1. Add `tinycld.org/core v0.0.0` to your package's `go.mod` with a replace directive (the audit subpackage lives inside core):
   ```
   require tinycld.org/core v0.0.0
   replace tinycld.org/core => ../../../server
   ```
2. Call `audit.RegisterCollection` in your `Register()` function.
3. Run `go mod tidy` and `go build ./...` to verify.
