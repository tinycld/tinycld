---
title: Installing packages
summary: How feature packages plug into the app shell
tags: [packages, install, extend]
order: 50
---

## Feature packages

The app shell is intentionally small. Mail, contacts, calendar, drive, and every other feature is a separate **package** that plugs in via a manifest.

Installed packages appear automatically in:

- The nav rail (one icon per package)
- Settings (each package can contribute its own settings panels)
- Help (each package can ship its own help topics — like this one)
- The data layer (each package registers its own PocketBase collections)

## Installing a new package

For developers, packages are linked into the app shell with:

```sh
npm run packages:install <git-url>
```

or, for a package already cloned locally:

```sh
npm run packages:link <slug>
```

The generator wires the new package into routes, the data layer, and the Go server automatically. Restart the dev server to pick up changes.

## Removing a package

```sh
npm run packages:unlink <package-name>
```

Removing a package hides its UI and stops registering its collections, but **does not delete data** — relinking the package restores everything.

## Customizing the nav icon

A package's nav-rail icon comes from `manifest.ts`:

```ts
nav: {
    label: 'My Package',
    icon: 'cloud-rain',
}
```

The value is any kebab-case icon name from [lucide.dev/icons](https://lucide.dev/icons) — copy it straight from the URL slug. The generator picks up the name on `npm install`, bundles just that icon, and the rail renders it everywhere. No edits to `@tinycld/core` are required to ship a new package with a new icon.

If the name doesn't match a real lucide icon, the generator fails loudly at install time with a "did you mean…?" hint. If an icon name slips through (for example, on a runtime-installed package whose manifest wasn't present when the app was last bundled), the rail falls back to a `?` placeholder until a rebuild includes it.
