# EAS workspace-root fallback files

These are **verbatim copies** of workspace-root files that the `tinycld/workspace`
repo owns but EAS does not clone (EAS only checks out the app shell and
`eas-install-packages.sh` reconstructs the workspace around it):

- `link-members.ts`     → copied to `<workspace-root>/scripts/link-members.ts`
- `tinycld.packages.ts` → copied to `<workspace-root>/tinycld.packages.ts`

`eas-install-packages.sh` copies them into a freshly-reconstructed workspace root
during the EAS post-install step so the root `postinstall` (link-members + the
generator, which imports `getPackages()` from `tinycld.packages.ts`) can run.

**Keep these in sync with the originals at the workspace root.** They must match
byte-for-byte; if you edit `~/code/tinycld/scripts/link-members.ts` or
`~/code/tinycld/tinycld.packages.ts`, re-copy them here.
