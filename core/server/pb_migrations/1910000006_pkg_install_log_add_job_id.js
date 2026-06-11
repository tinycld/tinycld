/// <reference path="../pb_data/types.d.ts" />
// Adds job_id so the /admin progress modal can poll an operation's durable
// outcome by its unique job id. The live SSE stream dies when a successful
// apply restarts the server (the new process has no in-memory job), so the row
// — finalized before the restart — is the only thing that survives to tell the
// UI the operation succeeded. Keying by job_id (vs slug) avoids the stale-row
// ambiguity of re-running an operation for the same package.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        collection.fields.add(
            new Field({
                id: 'pil_job_id',
                name: 'job_id',
                type: 'text',
                max: 100,
            })
        )
        collection.indexes = [
            ...collection.indexes,
            'CREATE INDEX `idx_pkg_install_log_job_id` ON `pkg_install_log` (`job_id`)',
        ]
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        collection.indexes = collection.indexes.filter(
            i => !i.includes('idx_pkg_install_log_job_id')
        )
        collection.fields.removeById('pil_job_id')
        app.save(collection)
    }
)
