/// <reference path="../pb_data/types.d.ts" />
// Let console admins READ pkg_install_log.
//
// The Packages screen adopts a package job that is already running — one
// started by another admin, or by this admin before a page reload — by
// watching the log for a `running` row and opening the progress panel on its
// job id (the SSE stream backfills the history). That watch is a live query
// against this collection, which shipped superuser-only; without the read
// rule an owner's page would never see the row. Reads only: rows are written
// by the Go install pipeline, never by a client. Mirrors pkg_build in
// 1970000000_admin_console_role_rules.
const ADMIN =
    '@request.auth.id != "" && @request.auth.disabled != true && ' +
    '(@request.auth.role = "owner" || @request.auth.role = "admin")'

migrate(
    app => {
        const log = app.findCollectionByNameOrId('pkg_install_log')
        log.listRule = ADMIN
        log.viewRule = ADMIN
        app.save(log)
    },
    app => {
        const log = app.findCollectionByNameOrId('pkg_install_log')
        log.listRule = null
        log.viewRule = null
        app.save(log)
    }
)
