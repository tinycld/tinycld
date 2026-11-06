/// <reference path="../pb_data/types.d.ts" />
// org_branding.brand_color: the color documents the apps print use for their
// heading, rules and table header. Optional: empty means the default (the
// email accent, mailer.BrandColor). Public read like the logo, for the same
// reason (the row's rules are unchanged). A new migration because
// 2020000001_create_org_branding.js is released.
migrate(
    app => {
        const col = app.findCollectionByNameOrId('org_branding')
        col.fields.add(
            new TextField({
                id: 'ob_brand_color',
                name: 'brand_color',
                max: 7,
                pattern: '^#[0-9a-fA-F]{6}$',
            })
        )
        app.save(col)
    },
    app => {
        const col = app.findCollectionByNameOrId('org_branding')
        col.fields.removeById('ob_brand_color')
        app.save(col)
    }
)
