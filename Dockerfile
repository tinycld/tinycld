# Build pipeline assumes the build context is the ASSEMBLED WORKSPACE ROOT:
#
#   <context>/                 # the pnpm workspace root (NOT a git repo)
#       package.json           # packageManager: pnpm@<ver>; tsx devDep; postinstall
#       pnpm-workspace.yaml     # member list + pnpm settings (nodeLinker: hoisted)
#       pnpm-lock.yaml          # pinned lockfile (from the release asset)
#       .npmrc                 # minimal (pnpm settings live in pnpm-workspace.yaml)
#       tinycld.packages.ts    # member enumeration used by the generator
#       scripts/link-members.ts # links members into node_modules/@tinycld/ (postinstall)
#       tests/                 # shared unit-test stubs
#       tinycld/               # the MERGED member: app shell at its root +
#           core/              #   @tinycld/core nested here (Go module tinycld.org/core)
#           package-scripts/   #   the tinycld-pkg CLI nested here
#           server/            #   the app's Go module tinycld.org/tinycld (generator output)
#           scripts/           #   the generator (scripts/generate.ts) + runtime scripts
#           app/ lib/ ...      #   Expo Router route tree + app source
#       contacts/ mail/ ...    # feature members (each its own sibling repo)
#
# Post-merge layout: the app shell, @tinycld/core, and @tinycld/package-scripts
# all live INSIDE the single `tinycld/` member (previously two separate `app/` +
# `core/` members). The generator is tinycld/scripts/generate.ts (run by the
# workspace-root `postinstall`). CI assembles this tree first (e.g.
# `@tinycld/bootstrap --assemble-only --with <features>`) and then builds with
# the workspace root as context:
#
#   docker build -f tinycld/Dockerfile -t tinycld <workspace-root>
#
# `pnpm install --frozen-lockfile` at the root installs the pinned graph; the
# postinstall runs the generator then link-members (linking every member under
# node_modules/@tinycld/<name> plus @tinycld/app-generated → tinycld/lib/generated).
# The generator materializes tinycld/lib/generated/, tinycld/tinycld.config.ts,
# tinycld/tinycld.seeds.ts, route re-exports under tinycld/app/a/[orgSlug]/<slug>/,
# tinycld/server/{package_extensions.go,go.work,pb_migrations/,pb_hooks/,
# bundled-packages.json}, etc. The Go app module is tinycld.org/tinycld at
# tinycld/server/ with `replace tinycld.org/core => ../core/server` and a
# generated go.work wiring each feature's ../../<x>/server.
#
# RUNTIME LAYOUT (rebuild-from-scratch model). The image bakes a PRISTINE first
# build at /opt/tinycld-baked; the entrypoint copies it to a build dir at first
# boot. Each build dir is a complete workspace; the `current` symlink selects the
# live one. Mutable state lives at /workspace, OUTSIDE the swapped build tree.
#   /opt/tinycld-baked/         pristine baked workspace (root manifests,
#                               node_modules, feature siblings, tinycld/ member)
#   /workspace/                 STATE root (resolveStateDir()): pb_data/, releases/,
#                               builds/, .pnpm-store/, current → builds/<id>/tinycld
#   /workspace/builds/<id>/     a complete build (a copy of the baked tree or one
#                               assembled by the in-app rebuild). Its tinycld/ holds
#                               the binary (resolveServerDir()), server/, lib/, app/.
#   /workspace/current/         symlink → the active build's tinycld/ (the binary's
#                               dir; goSrcDir = current/server, core at current/core)

# Pre-builder: compiles the standalone `export-types` Go binary used by the
# workspace postinstall (see tinycld/scripts/export-types.ts). The binary
# regenerates core/types/pbSchema.ts + pbZodSchema.ts from migrations so the
# subsequent `expo export` can typecheck every package's collections.ts /
# types.ts. It imports only core/coreserver — pure Go, no CGO, no feature-
# server dependency chain (mupdf, goheif, dav1d), so we don't drag a C
# toolchain into the lean web-builder Node stage just to write two TS files.
#
# Sources copied: tinycld/core/server only (the binary's full dependency closure).
# The go-builder stage below builds the real CGO_ENABLED Linux runtime binary
# from the full workspace; this stage is throwaway, ~50 lines of Go work.
FROM golang:1.25-trixie AS types-binary-builder
WORKDIR /src
COPY tinycld/core/server/ ./core/server/
WORKDIR /src/core/server
RUN CGO_ENABLED=0 go build -o /out/export-types ./cmd/export-types/

# Node stage: install the workspace (runs the generator), build the web app.
FROM node:22-bookworm-slim AS web-builder

WORKDIR /ws

# The pre-built export-types binary. tinycld/scripts/export-types.ts reads
# TINYCLD_EXPORT_TYPES_BIN and invokes the binary directly when set,
# skipping the `go run` toolchain dependency that would otherwise force a
# Go install in this Node-only stage.
COPY --from=types-binary-builder /out/export-types /usr/local/bin/export-types
ENV TINYCLD_EXPORT_TYPES_BIN=/usr/local/bin/export-types

# Root manifests + shared test stubs + the workspace-root scripts (link-members).
# Copied first so the subsequent member COPYs are the only thing that changes
# between most builds.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc tinycld.packages.ts ./
COPY scripts/ ./scripts/
COPY tests/ ./tests/

# Workspace members. The merged `tinycld/` member (app shell + nested core +
# nested package-scripts) first — it carries the generator and is most likely to
# change last — then the feature members. Each is a real directory in the
# assembled context; no symlinks, no per-member node_modules (pnpm install
# recreates the workspace links).
COPY tinycld/ ./tinycld/
COPY contacts/ ./contacts/
COPY mail/ ./mail/
COPY calendar/ ./calendar/
COPY drive/ ./drive/
COPY calc/ ./calc/
COPY text/ ./text/
COPY google-takeout-import/ ./google-takeout-import/

# expo-env.d.ts is an Expo-generated, gitignored type shim (a single triple-slash
# reference to expo/types). Because it's gitignored it isn't in the build
# context, so ensure it exists for the runtime COPY below (the in-app installer
# re-runs the generator + a web rebuild, which needs Expo's ambient types).
RUN [ -f tinycld/expo-env.d.ts ] || printf '/// <reference types="expo/types" />\n' > tinycld/expo-env.d.ts

# Pin the pnpm content-addressable store to a FIXED absolute path
# (/workspace/.pnpm-store) so the runtime in-app installer reuses the store baked
# into the image instead of re-fetching the entire ~2GB dependency graph from the
# network on every package install/upgrade.
#
# Why this is needed: the runtime image ships node_modules as real files, but the
# pnpm STORE (the content-addressable cache pnpm links node_modules from) is NOT
# carried into the runtime image by default — it lives under the BUILD user's HOME
# (/root/.local/share/pnpm). At runtime the installer runs `pnpm install` as the
# unprivileged tinycld user, whose store resolves to its own HOME
# (/workspace/tinycld/.local/share/pnpm) — EMPTY. So pnpm re-downloads all ~1200
# packages (reused 0, downloaded 1200) on every operation; on a slow/flaky network
# that is minutes-to-effectively-stalled, which is what made the todo-install
# upgrade phase appear to hang.
#
# The fix: a fixed store-dir set via pnpm-workspace.yaml's `storeDir:` key (the
# ONLY mechanism pnpm 10+ honors — .npmrc store-dir and npm_config_store_dir env
# are ignored), pinned to the SAME absolute path in both this build stage and the
# runtime stage so a single copied store satisfies both. We append it to the
# build's copy of pnpm-workspace.yaml (NOT the committed source — dev machines
# keep their default ~/.local store). /workspace/.pnpm-store is on the same
# filesystem as /workspace/node_modules, so pnpm hardlinks rather than copies.
RUN printf '\nstoreDir: /workspace/.pnpm-store\n' >> pnpm-workspace.yaml

# Bound pnpm's network waits so the runtime in-app installer can't hang forever.
# pnpm 11 verifies the lockfile against supply-chain policies and fetches package
# metadata at the start of every install; if one of those connections stalls
# mid-stream (established but idle — observed intermittently on slow CI networks),
# pnpm waits on it with NO effective timeout and the whole install hangs
# indefinitely (seen: 22 min stuck at "Running pnpm install", zero progress). A
# bounded fetchTimeout + a couple of retries turns an infinite hang into a
# fast-failing, retried request — the installer then surfaces a real error
# instead of appearing dead. Set in pnpm-workspace.yaml (the mechanism pnpm 10+
# honors) so all three in-app `pnpm install` call sites inherit it; appended to
# the build copy only, so the committed source / dev machines are untouched.
RUN printf 'fetchTimeout: 60000\nfetchRetries: 2\nfetchRetryMaxtimeout: 30000\n' >> pnpm-workspace.yaml

# Install the whole workspace at the root. The postinstall runs the generator
# (`cd tinycld && pnpm run packages:generate`) and link-members (linking every
# member under node_modules/@tinycld/<name> plus @tinycld/app-generated). All
# members are present by now, so the generator resolves every package. Do NOT
# pass --ignore-scripts: the postinstall IS the generation step we depend on for
# the Go wiring, route re-exports, and lib/generated/. corepack enable picks the
# pnpm version pinned in package.json; --frozen-lockfile enforces the pinned
# pnpm-lock.yaml for a reproducible image. The store lands in /workspace/.pnpm-store
# (storeDir above), ready to be copied into the runtime image.
RUN corepack enable && pnpm install --frozen-lockfile

# Resolve the migration/hook symlinks the generator wrote under
# tinycld/server/{pb_migrations,pb_hooks} into real files. They point at member
# source via node_modules symlinks here, which is fine for this stage, but the
# go-builder and runtime COPY steps need real content (a COPY of a symlink that
# escapes its tree breaks), so materialize them in place.
RUN for d in tinycld/server/pb_migrations tinycld/server/pb_hooks; do \
        [ -d "$d" ] || continue; \
        find "$d" -type l -exec sh -c 'target=$(readlink -f "$1") && rm "$1" && cp "$target" "$1"' _ {} \; ; \
    done

# Stage a tree containing only Go module manifests (go.mod / go.sum / go.work),
# directory structure preserved. The go-builder stage copies just this tree to
# warm its module cache, so `go mod download` only re-runs when one of these
# files changes — independent of any Go source edits. The app server's go.work
# is generator output; it wires the sibling Go modules (core + each feature's
# server/) into the workspace, so it must travel with the manifests. Scan the
# workspace members (tinycld/server, tinycld/core/server, <feature>/server).
# tar --files-from preserves relative paths.
RUN mkdir -p /ws/go-mod-staging \
    && cd /ws \
    && find tinycld contacts mail calendar drive calc text google-takeout-import \
        \( -name go.mod -o -name go.sum -o -name go.work -o -name go.work.sum \) -print0 \
        | tar --null --files-from=- -cf - \
        | tar -xf - -C /ws/go-mod-staging

# Build web app. EXPO_PUBLIC_* vars are inlined at bundle time, so they must be
# present in the environment when `expo export` runs. Pass them in via
# `docker build --build-arg`.
ARG EXPO_PUBLIC_SENTRY_DSN=
ARG EXPO_PUBLIC_GIT_COMMIT=
ENV EXPO_PUBLIC_ENV=web
ENV EXPO_PUBLIC_SENTRY_DSN=$EXPO_PUBLIC_SENTRY_DSN
ENV EXPO_PUBLIC_GIT_COMMIT=$EXPO_PUBLIC_GIT_COMMIT
ENV NODE_OPTIONS="--max-old-space-size=2048"

# Bring in .release-id, which the deploy tooling writes into the merged member
# right before pushing. Format: YYYY-MM-DD-HHMMSS-<short-sha>. When absent
# (a hand-run `docker build`), the RUN below derives an id internally.
COPY tinycld/.release-id* ./tinycld/

# Bring in .release-manifest, the pinned-release manifest the release pipeline
# uploads as a GitHub Release asset (utils/lib/pin-release.ts). CI copies it to
# tinycld/.release-manifest before `docker build`; the RUN below stages it next
# to release-id.txt so the Go /api/release handler can serve it. The wildcard
# makes it optional — local/hand-run builds with no manifest still build cleanly.
COPY tinycld/.release-manifest* ./tinycld/

# Resolve effective release id and stage the dist tree under
# tinycld/release-staging/<id>/. Done in one shell so the resolved id is
# consistent across all steps. The web build runs from the merged member
# (WORKDIR /ws/tinycld): Metro's watchFolders points at the workspace root, so
# member source resolves through the node_modules/@tinycld/* symlinks. The
# entrypoint promotes this staging dir on container start and renames the SPA
# shell index.html → app.html. EXPO_PUBLIC_RELEASE_ID is inlined so /api/version
# polling can detect deploys.
WORKDIR /ws/tinycld
RUN set -eu \
    && if [ -s .release-id ]; then \
        rid=$(tr -d '[:space:]' < .release-id); \
    else \
        sha="${EXPO_PUBLIC_GIT_COMMIT:-deadbeef}"; \
        sha=$(printf '%s' "$sha" | cut -c1-7); \
        case "$sha" in *[!a-f0-9]*) sha=deadbeef;; esac; \
        rid="$(date -u +%Y-%m-%d-%H%M%S)-$sha"; \
    fi \
    && rm -f .release-id \
    && export EXPO_PUBLIC_RELEASE_ID="$rid" \
    && npx expo export --platform web \
    && mkdir -p /ws/tinycld/release-staging \
    && mv /ws/tinycld/dist "/ws/tinycld/release-staging/$rid" \
    && printf '%s' "$rid" > "/ws/tinycld/release-staging/$rid/release-id.txt" \
    && if [ -s .release-manifest ]; then \
        cp .release-manifest "/ws/tinycld/release-staging/$rid/manifest.json"; \
        rm -f .release-manifest; \
    fi \
    && mv "/ws/tinycld/release-staging/$rid/index.html" "/ws/tinycld/release-staging/$rid/app.html"


# Build stage for Go server.
FROM golang:1.25-trixie AS go-builder

# Install CGo dependencies for mupdf (thumbnail generation via go-fitz).
RUN apt-get update \
    && apt-get install -y libmupdf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /ws

# Stage only go.mod / go.sum / go.work files first so the module-download layer
# caches until those files change. Without this, every source edit busts the
# cache and re-downloads ~hundreds of MB of Go modules. The app server's go.work
# (generator output) wires the sibling Go modules — core via the go.mod
# `replace => ../core/server`, each feature via go.work
# `use ../../<x>/server`. Those relative paths resolve from /ws/tinycld/server,
# so the manifests must land at their member paths. The web-builder staged them
# all under /ws/go-mod-staging/ with structure preserved.
COPY --from=web-builder /ws/go-mod-staging/ ./

# The go.work `use` entries reference ../../<x>/server (feature siblings) and the
# go.mod `replace` references ../core/server. The module-download step only needs
# the manifests + the directory structure to exist; the full source lands below.

WORKDIR /ws/tinycld/server
# Warm the module cache. Reused on every rebuild as long as none of the
# go.mod/go.sum/go.work files copied above changed.
RUN go mod download

# Now bring in the full member trees the Go workspace spans: the merged member
# (app server + nested core's Go module — the replace target), and each
# feature's source (its server/ is a go.work member; copying the whole member
# dir is simplest and the non-Go files are cheap). Changes here invalidate
# everything below but leave the (much larger) module-download layer above intact.
WORKDIR /ws
COPY --from=web-builder /ws/tinycld/ ./tinycld/
COPY --from=web-builder /ws/contacts/ ./contacts/
COPY --from=web-builder /ws/mail/ ./mail/
COPY --from=web-builder /ws/calendar/ ./calendar/
COPY --from=web-builder /ws/drive/ ./drive/
COPY --from=web-builder /ws/calc/ ./calc/
COPY --from=web-builder /ws/text/ ./text/
COPY --from=web-builder /ws/google-takeout-import/ ./google-takeout-import/

WORKDIR /ws/tinycld/server
# Reconcile go.sum across the workspace after the full source lands. The
# web-builder generated package_extensions.go and go.work but had no Go
# toolchain to populate go.sum entries; `go work sync` does that now, offline
# (module cache is warm from `go mod download` above) and only costs cycles
# when go.work or any sibling go.mod changed.
RUN if [ -f go.work ]; then go work sync; fi

# Build the server binary.
RUN CGO_ENABLED=1 GOOS=linux go build -o tinycld .


# Final runtime stage
FROM debian:trixie-slim AS runtime

ENV SENTRY_DSN=""
# Domain / TLS config consumed by entrypoint.sh:
#   PRIMARY_DOMAIN      canonical domain (cert primary SAN + user-facing URLs)
#   ADDITIONAL_DOMAINS  comma-separated extra cert domains
#   AUTOCERT_ENABLED    true/false — provision Let's Encrypt + bind :80/:443
#   PUBLIC_SCHEME       http|https for the user-facing URL in reverse-proxy mode
#                       (default https; ignored when autocert is on)
# Autocert turns on only when AUTOCERT_ENABLED=true AND PRIMARY_DOMAIN is set;
# otherwise the server runs plain HTTP on :7090 (reverse-proxy / local-demo).
ENV PRIMARY_DOMAIN=""
ENV ADDITIONAL_DOMAINS=""
ENV AUTOCERT_ENABLED=""
ENV PUBLIC_SCHEME=""
ENV FZ_VERSION="1.25.1"

# Install runtime dependencies + Node for runtime tasks. Cron jobs in bin/
# invoke `pnpm exec tsx scripts/<x>.ts` (reset-demo, seed-db), so Node must be on
# PATH. libcap2-bin (setcap) is needed at image build time below; we keep it
# available so operators on autocert who use the in-app package installer can
# manually re-apply the cap to a freshly-rebuilt binary. git is required by the
# in-app package installer: `npm pack <git-spec>` (e.g. github:owner/repo) clones
# the repo via git, so without it git-spec installs fail with `spawn git ENOENT`.
# gcc AND g++ are required to install a package that ships a Go server: the
# installer's checkGoBuildPrereqs() gate needs `go` (from the copied toolchain)
# plus a C compiler, and the server is built with CGO_ENABLED=1. The cgo set
# needs BOTH compilers — gcc for libmupdf (go-fitz) and g++ for goheif/libde265
# (HEIF decode, which shells out to g++). The build-stage go-builder gets both
# from the golang:trixie base (build-essential); the slim runtime base has
# neither, so add them explicitly. Without gcc, server packages are rejected at
# manifest validation ("requires Phase 3 support"); without g++, the runtime
# `go build` fails with `exec: "g++": executable file not found`. libmupdf-dev
# (already listed) supplies the cgo link target.
#
# sqlite3 (the CLI) is needed by the installer's database-backup step, which runs
# `sqlite3 <db> "VACUUM INTO '<backup>'"` for a consistent snapshot before
# swapping in the rebuilt binary. The Go server embeds a SQLite driver so the CLI
# was never otherwise required; without it the install fails at "Backing up
# database" with `exec: "sqlite3": not found`.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libffi8 libmupdf-dev libcap2-bin curl git gcc g++ sqlite3 gnupg gosu \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Make `pnpm` available on PATH for the in-app package installer, which runs
# `pnpm install` at the workspace root (step 7 of the install pipeline). Node
# ships corepack; `corepack enable` only creates a SHIM that lazily downloads
# pnpm on first use AND prompts for confirmation — which fails non-interactively
# inside the installer ("! Corepack is about to download …pnpm-11.3.0.tgz",
# exit 1). So we `corepack prepare … --activate` here to actually fetch + cache
# the pinned pnpm into the image at build time.
#
# COREPACK_HOME must be a shared, world-readable path: corepack caches the
# prepared pnpm under $COREPACK_HOME (default ~/.cache/node/corepack), and we
# prepare it here as root but the installer runs pnpm as the unprivileged
# tinycld user — root's HOME cache is unreadable to tinycld. Point COREPACK_HOME
# at /opt/corepack (chmod a+rX) and set the SAME env at runtime so the tinycld
# process resolves the same cache. COREPACK_ENABLE_DOWNLOAD_PROMPT=0 belt-and-
# suspenders against any later fetch blocking on a prompt. Version matches the
# root package.json `packageManager` pin.
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
ENV COREPACK_HOME=/opt/corepack
RUN corepack enable \
    && corepack prepare pnpm@11.3.0 --activate \
    && chmod -R a+rX /opt/corepack

# Copy Go toolchain from build stage (needed for the in-app package installer's
# runtime Go-package rebuilds).
COPY --from=go-builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

# Non-root runtime user. UID/GID 1000 matches the typical first non-root host
# user on Linux distros, so host-side pb_data/ files on a bind-mount are owned
# by the host user instead of root. Override at build time with
# --build-arg TINYCLD_UID=... if your host user has a different uid.
ARG TINYCLD_UID=1000
ARG TINYCLD_GID=1000
RUN groupadd --system --gid "$TINYCLD_GID" tinycld \
    && useradd --system --uid "$TINYCLD_UID" --gid "$TINYCLD_GID" \
        --home-dir /workspace --no-create-home --shell /usr/sbin/nologin tinycld

# The runtime app tree lives at /workspace/tinycld and mirrors the merged member:
# the server binary, the per-release web bundle, migrations, nested core, and the
# runtime scripts. The workspace-level pieces the runtime scripts and in-app
# installer need (node_modules, root manifests, tinycld.packages.ts, the feature
# siblings) live at /workspace (the parent of /workspace/tinycld), so that
# node_modules/@tinycld/<x> symlinks → ../../<x> resolve within /workspace and
# resolveServerDir()==/workspace/tinycld with wsRoot==/workspace.
# The image bakes the fully-assembled workspace as a PRISTINE first build under
# /opt/tinycld-baked (an unmounted path so a bind-mounted /workspace/builds can't
# shadow it). The whole tree is baked — root manifests + node_modules + every
# sibling member + the tinycld member — because node_modules/@tinycld/<x> are
# RELATIVE symlinks (../../<x>), so a build dir must contain the entire workspace,
# not just tinycld/. At first boot the entrypoint copies this into
# /workspace/builds/build-baked and points /workspace/current at its tinycld/.
# Mutable state (.pnpm-store, pb_data, releases, builds) lives at /workspace,
# OUTSIDE the swapped tree.
WORKDIR /opt/tinycld-baked/tinycld

# Compiled server binary, placed at the member root (resolveServerDir() returns
# the binary's own dir; the in-app rebuild writes the live binary at
# <buildDir>/tinycld/tinycld and runs the Go toolchain in <buildDir>/tinycld/server).
COPY --from=go-builder /ws/tinycld/server/tinycld ./tinycld

# Per-release web bundle, staged by the web-builder. The entrypoint promotes
# this on container start to /workspace/releases/. The public/ directory is
# reserved for the marketing website.
COPY --from=web-builder /ws/tinycld/release-staging /opt/tinycld-baked/tinycld/release-staging
RUN mkdir -p /opt/tinycld-baked/tinycld/public

# Workspace-root files for the runtime scripts + in-app package-install pipeline.
# These live at the workspace ROOT: the root package.json / pnpm-lock.yaml /
# pnpm-workspace.yaml / .npmrc, the hoisted node_modules (with the @tinycld/<x>
# member symlinks), tinycld.packages.ts, and scripts/ (link-members.ts — the
# in-app installer's postinstall re-runs it). They sit at /workspace (one
# directory above /workspace/tinycld) so the symlinks
# (node_modules/@tinycld/<x> → ../../<x>) still point at the members copied below.
COPY --from=web-builder /ws/package.json /ws/pnpm-lock.yaml /ws/pnpm-workspace.yaml /ws/.npmrc /opt/tinycld-baked/
COPY --from=web-builder /ws/tinycld.packages.ts /opt/tinycld-baked/tinycld.packages.ts
COPY --from=web-builder /ws/scripts /opt/tinycld-baked/scripts
COPY --from=web-builder /ws/tests /opt/tinycld-baked/tests
COPY --from=web-builder /ws/node_modules /opt/tinycld-baked/node_modules

# The pnpm content-addressable store, populated by the web-builder's
# `pnpm install` (pinned to /workspace/.pnpm-store via the storeDir append in that
# stage; the runtime pnpm-workspace.yaml copied above carries the SAME storeDir).
# Carrying it into the image is what lets the in-app installer's `pnpm install`
# relink from the store (reused N, downloaded 0) instead of re-downloading the
# whole dependency graph from the network on every package install/upgrade — the
# root cause of the todo-install upgrade phase appearing to hang. The whole-tree
# `chown -R tinycld:tinycld /workspace` below makes it readable+writable by the
# runtime user (pnpm adds newly-fetched packages here on a genuine version bump).
COPY --from=web-builder /workspace/.pnpm-store /workspace/.pnpm-store

# Feature siblings at /workspace/<x>. seed-db.ts (run by the reset-demo cron)
# imports ../tinycld.seeds → each @tinycld/<feature>/seed, resolved through the
# node_modules symlinks (node_modules/@tinycld/<x> → ../../<x>) into these member
# dirs. They sit at /workspace so the symlinks resolve
# (/workspace/node_modules/@tinycld/<x> → /workspace/<x>).
COPY --from=web-builder /ws/contacts /opt/tinycld-baked/contacts
COPY --from=web-builder /ws/mail /opt/tinycld-baked/mail
COPY --from=web-builder /ws/calendar /opt/tinycld-baked/calendar
COPY --from=web-builder /ws/drive /opt/tinycld-baked/drive
COPY --from=web-builder /ws/calc /opt/tinycld-baked/calc
COPY --from=web-builder /ws/text /opt/tinycld-baked/text
COPY --from=web-builder /ws/google-takeout-import /opt/tinycld-baked/google-takeout-import

# The merged member's sub-trees the runtime + in-app installer need (everything
# inside tinycld/ EXCEPT the binary and release-staging, which were placed above
# and are NOT part of the source member). Copied per-subpath from the go-builder
# stage — which has the full member from web-builder PLUS the compiled Go
# artifacts — so the regenerated Go wiring (server/go.work, package_extensions.go,
# go.mod/go.sum, the materialized pb_migrations/pb_hooks), nested core (core/, the
# go.mod replace target + boot-time core/types target), nested package-scripts,
# the generator (scripts/), generated config (tinycld.config.ts, tinycld.seeds.ts,
# lib/), and the app source (app/, configs) all land at /workspace/tinycld.
# The merged member's own node_modules — tiny (just the @tinycld/<x> symlink
# dir; all real deps are hoisted to the workspace-root node_modules). Metro
# resolves member packages through these relative symlinks
# (node_modules/@tinycld/core → ../../core, …/mail → ../../../mail), so they must
# be present at /workspace/tinycld/node_modules for the runtime in-app installer's
# `expo export` to resolve @tinycld/* before its own pnpm install re-links them.
COPY --from=go-builder /ws/tinycld/node_modules/ ./node_modules/
COPY --from=go-builder /ws/tinycld/server/ ./server/
COPY --from=go-builder /ws/tinycld/core/ ./core/
COPY --from=go-builder /ws/tinycld/package-scripts/ ./package-scripts/
COPY --from=go-builder /ws/tinycld/scripts/ ./scripts/
COPY --from=go-builder /ws/tinycld/lib/ ./lib/
COPY --from=go-builder /ws/tinycld/app/ ./app/
# plugins/ and modules/ are needed by the in-app installer's `expo export`:
# app.json lists `./plugins/with-app-updater.cjs`, which getConfig() resolves
# (a missing file fails config resolution before bundling even starts), and
# metro.config.cjs maps the `app-updater` specifier to modules/app-updater/
# (its web stub on web, index.ts on native) — so both subtrees must ship in the
# runtime image, not just the dev tree.
COPY --from=go-builder /ws/tinycld/plugins/ ./plugins/
COPY --from=go-builder /ws/tinycld/modules/ ./modules/
COPY --from=go-builder /ws/tinycld/public/ ./public/
COPY --from=go-builder /ws/tinycld/assets/ ./assets/
COPY --from=go-builder /ws/tinycld/tinycld.config.ts /ws/tinycld/tinycld.seeds.ts ./
# tsconfig.package-base.json is NOT at the merged-member root post-merge — it
# lives in core (core/tsconfig.package-base.json, surfaced via @tinycld/core's
# exports) and arrives with the `core/` COPY above. Members extend it by package
# name, so no root copy is needed.
COPY --from=go-builder /ws/tinycld/package.json /ws/tinycld/tsconfig.json ./
COPY --from=go-builder /ws/tinycld/metro.config.cjs /ws/tinycld/babel.config.cjs ./
COPY --from=go-builder /ws/tinycld/app.json /ws/tinycld/global.css /ws/tinycld/uniwind-types.d.ts /ws/tinycld/expo-env.d.ts ./

# Point the runtime migrations dir at the generator/installer's migrations dir.
# jsvm reads /workspace/tinycld/pb_migrations; the generator (and the in-app
# installer when it regenerates after installing a package) writes migration
# symlinks into /workspace/tinycld/server/pb_migrations. Symlinking the former to
# the latter means a newly-installed package's migrations are immediately visible
# to the runtime and actually apply on the post-install restart — without this,
# installed-package migrations land only in server/pb_migrations and silently
# never run. Build-time (bundled) migrations live in server/pb_migrations too
# (resolved real files from the go-builder COPY above), so this unifies build +
# runtime + installer on one directory.
RUN rm -rf ./pb_migrations && ln -s server/pb_migrations ./pb_migrations

# bundled-packages.json so core's coreserver.SyncBundledPackages can find it at
# startup. Generated at tinycld/server/bundled-packages.json; the binary reads it
# relative to its own dir, so place a copy at /workspace/tinycld for the boot-time seed.
COPY --from=go-builder /ws/tinycld/server/bundled-packages.json ./bundled-packages.json

# Mutable state dirs at the workspace state root (resolveStateDir()==/workspace):
# pb_data (the live DB), releases (promoted web bundles), builds (per-build trees).
# These live OUTSIDE the swapped baked tree so they persist across the `current`
# symlink flip. The generated-types dir is code-adjacent and stays in the build
# tree (coreserver.DefaultTypesDir() → <binaryDir>/core/types).
RUN mkdir -p /workspace/pb_data /workspace/releases /workspace/builds \
    && mkdir -p /opt/tinycld-baked/tinycld/core/types

# The entrypoint lives at a FIXED path outside the swapped tree so it survives a
# `current` flip and isn't part of any build dir.
COPY tinycld/config/entrypoint.sh /opt/entrypoint.sh
RUN chmod +x /opt/entrypoint.sh

# Hand the workspace + baked tree to the tinycld user with a single build-time
# chown. Because these are ordinary subdirectories (not the overlay mount root /),
# the chown PERSISTS through the layer commit — unlike a chown of / which reverts
# to root:root. So no runtime chown is needed for these paths; only bind-mounted
# data dirs (/workspace/pb_data, /workspace/builds, /workspace/releases) need
# fixing at runtime when Docker creates them owned by root.
#
# Must run BEFORE setcap below — chown strips file capabilities (it resets the
# security.capability xattr along with ownership), so setcap'ing first then
# chown'ing would silently wipe the cap.
RUN chown -R tinycld:tinycld /workspace /opt/tinycld-baked /opt/entrypoint.sh

# Grant cap_net_bind_service so the non-root user can bind :80/:443 when
# autocert is on (AUTOCERT_ENABLED=true with PRIMARY_DOMAIN set). The plain-HTTP
# path defaults to the unprivileged :7090 and needs no special permissions.
#
# Caveat: the in-app package installer rebuilds the binary with `go build` and
# os.Renames it into place. The new binary has no caps. On autocert hosts that
# use the installer, the operator needs to re-apply the cap manually (the image
# ships setcap for this) or restart from the original image. Plain HTTP is fine.
RUN setcap 'cap_net_bind_service=+ep' /opt/tinycld-baked/tinycld/tinycld

# 7090: plain HTTP (default, when autocert is off)
# 80:   autocert HTTP-01 challenge + plain-HTTP redirect (when autocert is on)
# 443:  autocert HTTPS (when autocert is on)
# 993:  IMAPS (implicit TLS)
# 465:  SMTPS (implicit TLS)
EXPOSE 7090 80 443 993 465

# The container starts as root so the entrypoint can fix ownership of the
# bind-mounted data dirs (/workspace/pb_data, /workspace/builds,
# /workspace/releases) before the server runs. When a host bind-mount
# target doesn't exist yet, Docker creates it owned by root; the unprivileged
# tinycld user then can't open the SQLite DB ("unable to open database file
# (14)") and the container crash-loops. entrypoint.sh chown's those dirs to
# tinycld and drops to uid 1000 via gosu for the server itself, so nothing
# privileged actually runs the application. See fix_data_dir_ownership() in
# entrypoint.sh.
USER root

# The server process still runs as uid 1000 (tinycld) — the entrypoint drops
# privileges with gosu before exec'ing it. The binary's cap_net_bind_service
# file capability lets that unprivileged process bind :80/:443 when autocert is
# enabled; the plain-HTTP default of :7090 is unprivileged.
#
# Set AUTOCERT_ENABLED=true with PRIMARY_DOMAIN (and optional comma-separated
# ADDITIONAL_DOMAINS) to serve with autocert (binds :80 + :443 directly,
# terminates TLS in-process):
#   dokku config:set myapp AUTOCERT_ENABLED=true PRIMARY_DOMAIN=tinycld.org \
#     ADDITIONAL_DOMAINS="tinycld.com,www.tinycld.org"
# Otherwise serve plain HTTP on :7090 (override with HTTP_ADDR), expecting an
# upstream reverse proxy or compose port mapping to route to it. PRIMARY_DOMAIN
# still feeds the user-facing setup URL in plain-HTTP/proxy mode.
ENTRYPOINT ["/opt/entrypoint.sh"]
