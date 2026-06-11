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

## Trust and security

Installing a package runs that package's code on your server with full access to
your data. A package isn't passive data the app just reads — its server code is
compiled into the running server and executes in-process, with the same privileges
as the rest of the deployment, every time the server starts. Installing one is the
same kind of decision as choosing what code to run on your own machine: only
install packages from authors you trust, just as you would with `npm install`.

Only super admins can install or update packages, and that restriction is enforced
on the server. Packages outside the official `@tinycld/` scope show an extra
caution notice at install time — but the warning is advisory, so the judgment about
whether to trust a source is yours.

## Installing a new package

For developers, packages are linked into the app shell with:

```sh
pnpm run packages:install <git-url>
```

or, for a package already cloned locally:

```sh
pnpm run packages:link <slug>
```

The generator wires the new package into routes, the data layer, and the Go server automatically. Restart the dev server to pick up changes.

## Removing a package

```sh
pnpm run packages:unlink <package-name>
```

Removing a package hides its UI and stops registering its collections, but **does not delete data** — relinking the package restores everything.

## Checking which versions are installed

To see exactly which version of each package your app is running, open **Settings → About**. Below the app version, an **Included packages** list shows every bundled package with its released version and the short commit it was built from, plus the date the release was assembled.

This list reflects the pinned release the running image was built from, so it's the authoritative answer to "what's actually deployed". It appears only on released builds — local development installs show just the app version.

## Customizing the nav icon

A package's nav-rail icon comes from `manifest.ts`:

```ts
nav: {
    label: 'My Package',
    icon: 'cloud-rain',
}
```

The value is any kebab-case icon name from [lucide.dev/icons](https://lucide.dev/icons) — copy it straight from the URL slug. The generator picks up the name on `pnpm install`, bundles just that icon, and the rail renders it everywhere. No edits to `@tinycld/core` are required to ship a new package with a new icon.

If the name doesn't match a real lucide icon, the generator fails loudly at install time with a "did you mean…?" hint. If an icon name slips through (for example, on a runtime-installed package whose manifest wasn't present when the app was last bundled), the rail falls back to a `?` placeholder until a rebuild includes it.
