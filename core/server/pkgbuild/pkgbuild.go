// Package pkgbuild is the shared package-build pipeline: fetch + validate
// workspace members, assemble an out-of-tree pnpm workspace, and drive it
// through pnpm install / go build / expo export into a runtime tree.
//
// It is consumed by two hosts (see multi-org/docs/DESIGN-org-package-agency.md,
// D5): the single-tenant coreserver, which wraps it with its DB-backed tails
// (pkg_registry, install log, DB backup, symlink activate, exit-75 restart),
// and the multi-org trusted builder, which wraps it with job intake, per-job
// confinement, and a content-addressed artifact cache.
//
// Two hard rules keep it shareable:
//
//   - No host imports. pkgbuild never imports coreserver or PocketBase. Hosts
//     inject behavior through the seams: MemberSource (fetch vs
//     copy-from-current), ProgressSink (SSE/install-log vs job log), and
//     CmdRunner/StreamingRunner (in-process exec vs jailed child).
//
//   - Driver, not toolchain. The pipeline delegates the real work to the
//     FETCHED workspace's own scripts (postinstall runs the build dir's
//     generator, not the host's). This is what lets one builder build
//     workspaces pinned to different core versions.
package pkgbuild
