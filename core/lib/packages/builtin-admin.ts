import type { PackageManifest } from './types'

// The Admin area renders in the workspace shell like a package — rail-reachable,
// with a PackageSidebar and an active-slug — but it is NOT an installed package:
// it has no manifest, no member dir, and must never appear among the regular
// package rail icons (it has its own super-admin-gated rail button instead). To
// satisfy the shell's package-shaped lookups (usePackage(slug)?.sidebar and
// PackageSidebar's packageSidebars[slug]) without faking a generated package, we
// expose this single synthetic entry and inject it ONLY into usePackage — never
// into usePackages()/useSortedPackages(), so it stays out of the rail list.

export const ADMIN_PACKAGE_SLUG = 'admin'

// `sidebar.component` is a marker only — the actual component is wired by slug in
// derive-components' packageSidebars. The shell just needs `.sidebar` non-null.
export const ADMIN_PACKAGE_ENTRY: PackageManifest & { packageName: string } = {
    name: 'Admin',
    slug: ADMIN_PACKAGE_SLUG,
    version: '0.0.0',
    description: 'Deployment administration',
    sidebar: { component: 'builtin' },
    packageName: '@tinycld/core',
}
