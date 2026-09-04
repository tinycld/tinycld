/// <reference path="../pb_data/types.d.ts" />
// Generalize `comment_mentions` from a drive-only table to a polymorphic one,
// so packages that don't store their content as drive items (boards is the
// first) can use the same mentions → notify pipeline.
//
// The table is created by @tinycld/drive
// (drive/pb-migrations/1781000000_create_comment_mentions.js) with
// `drive_item` as a REQUIRED relation, and its createRule authorizes THROUGH
// that relation:
//
//     @request.auth.id != "" && drive_item.drive_shares_via_item.user ?= @request.auth.id
//
// A cards mention is therefore both unrepresentable (no drive item exists to
// point at) and unauthorizable (the only rule branch resolves through drive).
// Drive's header called that coupling deliberate — "the entire
// comments-with-mentions feature is gated on documents being stored as drive
// items". Cards is the case that retires the assumption.
//
// WHAT THIS MIGRATION DOES *NOT* DO: it does not add an authorization branch
// for any package. It cannot. PocketBase's rule validator resolves every
// `@collection.<name>` reference eagerly at save time and REJECTS the whole
// rule if one is missing — verified directly, including for an OR-ed rule
// where only one branch is absent (the validator does not short-circuit).
// Because migrations are symlinked into a single flat directory from the
// INSTALLED packages only (tinycld/scripts/generate.ts), a core migration
// naming `boards_cards` would hard-fail at boot in every workspace that has no
// cards — breaking the lean-shell guarantee.
//
// So the work is split at the only seam that holds:
//   - core (here): the SHAPE. New target columns, `drive_item` relaxed,
//     existing rows backfilled. Names no feature collection, safe everywhere.
//   - each package: its own OR branch, appended by its own migration, which
//     ships only when that package ships and may therefore name its own
//     collections.
//
// Consequence for package authors: the createRule grows by CONCATENATION and
// feature migrations have no ordering guarantee between them, so a package
// must READ the current rule and append to it — never overwrite a hardcoded
// string, which would silently drop another package's branch.
//
// `comment_mentions` belongs to drive, which may be absent. Every step is
// therefore guarded: with no drive installed there is no table to alter and
// this migration is a no-op.
migrate(
    app => {
        let mentions
        try {
            mentions = app.findCollectionByNameOrId('comment_mentions')
        } catch {
            // No @tinycld/drive in this workspace — nothing to generalize.
            // A package that wants mentions without drive creates the table
            // itself; this migration only adapts drive's copy when present.
            return
        }

        // target_collection / target_record mirror the existing
        // comment_collection / comment_record pair: plain text, because
        // PocketBase does not model polymorphic relations. The Go notify hook
        // validates `comment_collection` against an allowlist before it
        // notifies, so a row naming an unknown target is dropped server-side.
        //
        // Sized to match their non-polymorphic counterparts: a collection
        // name is bounded well under 64, a record id is 15 chars today and
        // 32 leaves room.
        if (!mentions.fields.getByName('target_collection')) {
            mentions.fields.add(
                new TextField({
                    id: 'cm_target_collection',
                    name: 'target_collection',
                    max: 64,
                    // Not required: existing drive rows predate the column and
                    // the backfill below fills them in the same transaction.
                    // Requiring it here would reject those rows on save.
                    required: false,
                })
            )
        }
        if (!mentions.fields.getByName('target_record')) {
            mentions.fields.add(
                new TextField({
                    id: 'cm_target_record',
                    name: 'target_record',
                    max: 32,
                    required: false,
                })
            )
        }

        // Relax drive_item. It stays a real relation (with its cascadeDelete,
        // which is what removes a document's mentions when the document dies)
        // — it simply no longer applies to rows whose target isn't a drive
        // item. Drive's own insert path is untouched and keeps setting it.
        const driveItem = mentions.fields.getById('cm_drive_item')
        if (driveItem) {
            driveItem.required = false
        }

        app.save(mentions)

        // Backfill BEFORE anything reads the new columns, so they are
        // authoritative from here on and consumers need only one code path.
        // Existing rows are drive rows by definition — the table had no other
        // possible target.
        app.db()
            .newQuery(
                "UPDATE comment_mentions" +
                    " SET target_collection = 'drive_items', target_record = drive_item" +
                    " WHERE target_collection IS NULL OR target_collection = ''"
            )
            .execute()
    },
    app => {
        let mentions
        try {
            mentions = app.findCollectionByNameOrId('comment_mentions')
        } catch {
            return
        }

        // Restoring `required` on drive_item would reject any row a package
        // inserted with a non-drive target, so the down migration drops those
        // rows first. They are notification provenance, not user content —
        // the notifications they produced are separate records and survive.
        app.db()
            .newQuery(
                "DELETE FROM comment_mentions WHERE drive_item IS NULL OR drive_item = ''"
            )
            .execute()

        const driveItem = mentions.fields.getById('cm_drive_item')
        if (driveItem) {
            driveItem.required = true
        }
        mentions.fields.removeById('cm_target_collection')
        mentions.fields.removeById('cm_target_record')
        app.save(mentions)
    }
)
