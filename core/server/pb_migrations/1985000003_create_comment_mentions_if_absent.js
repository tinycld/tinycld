/// <reference path="../pb_data/types.d.ts" />
//
// Core owns the creation of the shared `comment_mentions` table.
//
// It began life in @tinycld/drive (1781000000): the original createRule
// authorized through the drive_item relation, and PocketBase's rule parser
// needs the referenced collections to exist at rule-save time, so core could
// not have shipped that rule. The generalization (1985000002) removed the
// reason the TABLE had to live there — target_collection / target_record
// carry the polymorphic target and drive_item is optional — but creation
// still rode drive, which quietly made every other package's mentions (and
// their test suites, and their CI) depend on drive being installed. A
// sibling-on-sibling dependency is exactly what the ecosystem forbids: a
// feature package may depend on core and nothing else.
//
// So core creates the table whenever no package has yet: the generalized
// shape, WITHOUT drive_item (a relation to a collection that may not exist
// here), and with every rule null — superusers only, until a package appends
// its own presence-gated createRule branch (boards' 1986000000 is the
// pattern; drive's create-or-adapt does the same when it arrives second).
// On a deployment where drive already created the table this is a no-op.
//
// The collection ID matches drive's create exactly, so whichever file runs
// first, every by-id reference resolves to the same collection.
//
// Numbered 1985000003: after the generalization (on drive-present
// deployments the adapt has already run, making this a clean no-op) and
// before every package's branch-append (boards' is 1986000000), which needs
// the columns created here.
migrate(
    app => {
        try {
            app.findCollectionByNameOrId('comment_mentions')
            // A package (drive, historically) already created it.
            return
        } catch {
            // Absent — core owns creation.
        }

        const commentMentions = new Collection({
            id: 'pbc_comment_mentions_01',
            name: 'comment_mentions',
            type: 'base',
            system: false,
            fields: [
                {
                    id: 'cm_comment_collection',
                    name: 'comment_collection',
                    type: 'text',
                    required: true,
                    max: 64,
                },
                {
                    id: 'cm_comment_record',
                    name: 'comment_record',
                    type: 'text',
                    required: true,
                    max: 32,
                },
                // Not required, matching what 1985000002 produces on drive's
                // copy: rows written before the generalization carry ''.
                {
                    id: 'cm_target_collection',
                    name: 'target_collection',
                    type: 'text',
                    required: false,
                    max: 64,
                },
                {
                    id: 'cm_target_record',
                    name: 'target_record',
                    type: 'text',
                    required: false,
                    max: 32,
                },
                {
                    id: 'cm_mentioned_user',
                    name: 'mentioned_user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'cm_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
            ],
            // Opaque to clients except for the create path, which each package
            // authorizes for itself: a null createRule reads as superuser-only
            // until the first branch is appended (appenders treat null and ''
            // alike — `mentions.createRule || ''`).
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            indexes: [
                'CREATE INDEX `idx_comment_mentions_target` ON `comment_mentions` (`comment_collection`, `comment_record`)',
                'CREATE INDEX `idx_comment_mentions_user` ON `comment_mentions` (`mentioned_user`, `created` DESC)',
            ],
        })
        app.save(commentMentions)
    },
    app => {
        // Remove only what this migration created. When drive created the
        // table this was a no-op on the way up and must stay one on the way
        // down; drive's copy is distinguishable by the drive_item field only
        // its create adds.
        let mentions
        try {
            mentions = app.findCollectionByNameOrId('comment_mentions')
        } catch {
            return
        }
        if (!mentions.fields.getByName('drive_item')) {
            app.delete(mentions)
        }
    }
)
