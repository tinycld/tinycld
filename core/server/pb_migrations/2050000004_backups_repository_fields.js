/// <reference path="../pb_data/types.d.ts" />
// Backups can go to a repository (the archive, or a deduplicating store such
// as PBS). The row records which one, the repository's own reference to the
// snapshot, and how many bytes were actually sent — for a deduplicating
// repository that is far less than `bytes`.
migrate(
    app => {
        const backups = app.findCollectionByNameOrId('backups')

        backups.fields.addAt(
            backups.fields.length,
            new Field({
                id: 'bk_repository',
                name: 'repository',
                type: 'text',
                max: 40,
            })
        )
        backups.fields.addAt(
            backups.fields.length,
            new Field({
                id: 'bk_ref',
                name: 'ref',
                type: 'text',
                max: 500,
            })
        )
        backups.fields.addAt(
            backups.fields.length,
            new Field({
                id: 'bk_uploaded_bytes',
                name: 'uploaded_bytes',
                type: 'number',
                min: 0,
            })
        )

        app.save(backups)
    },
    app => {
        const backups = app.findCollectionByNameOrId('backups')
        backups.fields.removeById('bk_repository')
        backups.fields.removeById('bk_ref')
        backups.fields.removeById('bk_uploaded_bytes')
        app.save(backups)
    }
)
