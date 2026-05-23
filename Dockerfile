# Build pipeline assumes the build context is the ASSEMBLED WORKSPACE ROOT:
#
#   <context>/                 # the npm workspace root (@tinycld/workspace)
#       package.json           # workspaces: [app, app/package-scripts, core, <features>]
#       package-lock.json
#       .npmrc
#       tinycld.packages.ts    # member enumeration used by the generator
#       tests/                 # shared unit-test stubs
#       core/                  # @tinycld/core — shared lib + Go module tinycld.org/core
#       app/                   # @tinycld/app — this shell (generator lives here)
#           package-scripts/   # the tinycld-pkg CLI (a workspace member, nested in app)
#       contacts/ mail/ ...    # feature members (each its own sibling repo)
#
# There is NO bundled `packages/@tinycld/core` and NO `generate-packages.ts`.
# core is a standalone sibling member, and the generator is app/scripts/generate.ts
# (run by the workspace-root `postinstall`). CI assembles this tree first
# (e.g. `@tinycld/bootstrap --tooling --with <features>`) and then builds with
# the workspace root as context:
#
#   docker build -f app/Dockerfile -t tinycld <workspace-root>
#
# `npm ci` at the root links every member under node_modules/@tinycld/<name>
# and runs the generator on postinstall, which materializes app/lib/generated/,
# app/tinycld.config.ts, app/tinycld.seeds.ts, route re-exports under
# app/app/a/[orgSlug]/<slug>/, app/server/{package_extensions.go,go.work,
# pb_migrations/,pb_hooks/,bundled-packages.json}, etc. The Go app module is
# tinycld.org/app at app/server/ with `replace tinycld.org/core => ../../core/server`
# and a generated go.work wiring each feature's ../../node_modules/@tinycld/<x>/server.

# Node stage: install the workspace (runs the generator), build the web app.
FROM node:22-bookworm-slim AS web-builder

WORKDIR /ws

# Root manifests + shared test stubs. Copied first so the subsequent member
# COPYs are the only thing that changes between most builds. package-scripts (the
# tinycld-pkg CLI) now lives inside the app member (app/package-scripts), so it
# arrives with the `COPY app/ ./app/` below — no separate root COPY.
COPY package.json package-lock.json .npmrc tinycld.packages.ts ./
COPY tests/ ./tests/

# Workspace members. core + app first (most likely to change last), then the
# feature members. Each is a real directory in the assembled context — no
# symlinks, no per-member node_modules (npm ci recreates the workspace links).
COPY core/ ./core/
COPY app/ ./app/
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
RUN [ -f app/expo-env.d.ts ] || printf '/// <reference types="expo/types" />\n' > app/expo-env.d.ts

# Install the whole workspace at the root. npm links every member under
# node_modules/@tinycld/<name> and then runs the workspace-root `postinstall`
# (`cd app && npm run packages:generate && npm run assets:copy-pdfjs`), which
# is the generator. All members are present by now, so the generator resolves
# every package. Do NOT pass --ignore-scripts: postinstall IS the generation
# step we depend on for the Go wiring, route re-exports, and lib/generated/.
RUN npm ci

# Resolve the migration/hook symlinks the generator wrote under
# app/server/{pb_migrations,pb_hooks} into real files. They point at member
# source via node_modules symlinks here, which is fine for this stage, but the
# go-builder and runtime COPY steps need real content (a COPY of a symlink that
# escapes its tree breaks), so materialize them in place.
RUN for d in app/server/pb_migrations app/server/pb_hooks; do \
        [ -d "$d" ] || continue; \
        find "$d" -type l -exec sh -c 'target=$(readlink -f "$1") && rm "$1" && cp "$target" "$1"' _ {} \; ; \
    done

# Stage a tree containing only Go module manifests (go.mod / go.sum / go.work),
# directory structure preserved. The go-builder stage copies just this tree to
# warm its module cache, so `go mod download` only re-runs when one of these
# files changes — independent of any Go source edits. The app server's go.work
# is generator output; it wires the sibling Go modules (core + each feature's
# server/) into the workspace, so it must travel with the manifests. Scan the
# workspace members (app/server, core/server, <feature>/server) rather than the
# old `server packages` roots. tar --files-from preserves relative paths.
RUN mkdir -p /ws/go-mod-staging \
    && cd /ws \
    && find app core contacts mail calendar drive calc text google-takeout-import \
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

# Bring in .release-id, which the deploy tooling writes into the app member
# right before pushing. Format: YYYY-MM-DD-HHMMSS-<short-sha>. When absent
# (a hand-run `docker build`), the RUN below derives an id internally.
COPY app/.release-id* ./app/

# Resolve effective release id and stage the dist tree under
# app/release-staging/<id>/. Done in one shell so the resolved id is consistent
# across all steps. The web build runs from the app member (WORKDIR /ws/app):
# Metro's watchFolders points at the workspace root, so member source resolves
# through the node_modules/@tinycld/* symlinks. The entrypoint promotes this
# staging dir on container start and renames the SPA shell index.html → app.html.
# EXPO_PUBLIC_RELEASE_ID is inlined so /api/version polling can detect deploys.
WORKDIR /ws/app
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
    && mkdir -p /ws/app/release-staging \
    && mv /ws/app/dist "/ws/app/release-staging/$rid" \
    && printf '%s' "$rid" > "/ws/app/release-staging/$rid/release-id.txt" \
    && mv "/ws/app/release-staging/$rid/index.html" "/ws/app/release-staging/$rid/app.html"


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
# `replace => ../../core/server`, each feature via go.work
# `use ../../node_modules/@tinycld/<x>/server`. Those relative paths resolve
# from /ws/app/server, so the manifests must land at their member paths. The
# web-builder staged them all under /ws/go-mod-staging/ with structure preserved.
COPY --from=web-builder /ws/go-mod-staging/ ./

# The go.work `use` entries reference ../../node_modules/@tinycld/<x>/server.
# Recreate those workspace symlinks (npm-owned in web-builder) so `go mod
# download`/`go work sync` can resolve each feature module through them. The
# symlink targets (the member server/ trees) land with the full source COPY
# below; for the cache-warming download step only the manifests + link graph
# need to exist.
COPY --from=web-builder /ws/node_modules/@tinycld ./node_modules/@tinycld

WORKDIR /ws/app/server
# Warm the module cache. Reused on every rebuild as long as none of the
# go.mod/go.sum/go.work files copied above changed.
RUN go mod download

# Now bring in the full member trees the Go workspace spans: the app server,
# core's Go module (replace target), and each feature's source (its server/ is
# a go.work member; copying the whole member dir is simplest and the non-Go
# files are cheap). Changes here invalidate everything below but leave the
# (much larger) module-download layer above intact.
WORKDIR /ws
COPY --from=web-builder /ws/app/ ./app/
COPY --from=web-builder /ws/core/ ./core/
COPY --from=web-builder /ws/contacts/ ./contacts/
COPY --from=web-builder /ws/mail/ ./mail/
COPY --from=web-builder /ws/calendar/ ./calendar/
COPY --from=web-builder /ws/drive/ ./drive/
COPY --from=web-builder /ws/calc/ ./calc/
COPY --from=web-builder /ws/text/ ./text/
COPY --from=web-builder /ws/google-takeout-import/ ./google-takeout-import/

WORKDIR /ws/app/server
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
ENV SERVE_ON_DOMAINS=""
ENV FZ_VERSION="1.25.1"

# Install runtime dependencies + Node for runtime tasks. Cron jobs in bin/
# invoke `npx tsx scripts/<x>.ts` (reset-demo, seed-db), so Node must be on
# PATH. libcap2-bin (setcap) is needed at image build time below; we keep it
# available so operators on autocert who use the in-app package installer can
# manually re-apply the cap to a freshly-rebuilt binary.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libffi8 libmupdf-dev libcap2-bin curl gnupg \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

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
        --home-dir /app --no-create-home --shell /usr/sbin/nologin tinycld

# The runtime app tree lives at /app and mirrors the app member: the server
# binary, the per-release web bundle, migrations, and the runtime scripts. The
# workspace-level pieces the runtime scripts and in-app installer need
# (node_modules, root manifests, tinycld.packages.ts, the sibling members) live
# one level up at /app/.. (i.e. /), assembled below so that
# node_modules/@tinycld/<x> symlinks → ../../<x> still resolve.
WORKDIR /app

# Compiled server binary.
COPY --from=go-builder /ws/app/server/tinycld ./tinycld

# Per-release web bundle, staged by the web-builder. The entrypoint promotes
# this on container start to /app/releases/. The /app/public/ directory is
# reserved for the marketing website (populated by tinycld.org's deploy tail).
COPY --from=web-builder /ws/app/release-staging /app/release-staging
RUN mkdir -p /app/public /app/releases

# Migrations with symlinks already resolved in web-builder.
COPY --from=web-builder /ws/app/server/pb_migrations ./pb_migrations

# Workspace-root files for the runtime scripts + in-app package-install
# pipeline. These live at the workspace ROOT in the new layout: the root
# package.json/lock/.npmrc, the hoisted node_modules (with the @tinycld/<x>
# member symlinks), and tinycld.packages.ts. They sit one directory above the
# app tree so the symlinks (node_modules/@tinycld/<x> → ../../<x>) still point
# at the sibling members copied below.
COPY --from=web-builder /ws/package.json /ws/package-lock.json /ws/.npmrc /
COPY --from=web-builder /ws/tinycld.packages.ts /tinycld.packages.ts
COPY --from=web-builder /ws/node_modules /node_modules
# package-scripts (the tinycld-pkg CLI) now lives inside the app member, so the
# node_modules/@tinycld/package-scripts symlink resolves to ../../app/package-scripts
# (i.e. /app/package-scripts at runtime, WORKDIR /app). Land it there.
COPY --from=web-builder /ws/app/package-scripts ./package-scripts

# Sibling members. seed-db.ts (run by the reset-demo cron) imports
# ../tinycld.seeds → each @tinycld/<feature>/seed, resolved through the
# node_modules symlinks (node_modules/@tinycld/<x> → ../../<x>) into these
# member dirs; core supplies the runtime libs they import. They sit at / so
# the symlinks resolve (/node_modules/@tinycld/<x> → /<x>).
COPY --from=web-builder /ws/core /core
COPY --from=web-builder /ws/contacts /contacts
COPY --from=web-builder /ws/mail /mail
COPY --from=web-builder /ws/calendar /calendar
COPY --from=web-builder /ws/drive /drive
COPY --from=web-builder /ws/calc /calc
COPY --from=web-builder /ws/text /text
COPY --from=web-builder /ws/google-takeout-import /google-takeout-import

# app-member files the runtime needs:
#   - tinycld.config.ts / tinycld.seeds.ts: imported by seed-db.ts at runtime.
#   - scripts/: reset-demo, seed-db, generate.ts + gen-*.ts (the in-app
#     installer re-runs the generator after installing a package).
#   - lib/generated/: package-help.ts etc. that the generated config imports.
#   - app config files (package.json, tsconfig, metro/babel, app.json,
#     global.css, public/, assets/): needed when the in-app installer re-runs
#     the generator + a web rebuild at runtime.
COPY --from=web-builder /ws/app/tinycld.config.ts /ws/app/tinycld.seeds.ts ./
COPY --from=web-builder /ws/app/package.json /ws/app/tsconfig.json /ws/app/tsconfig.package-base.json ./
COPY --from=web-builder /ws/app/metro.config.cjs /ws/app/babel.config.cjs /ws/app/react-native.config.cjs ./
COPY --from=web-builder /ws/app/app.json /ws/app/global.css /ws/app/uniwind-types.d.ts /ws/app/expo-env.d.ts ./
COPY --from=web-builder /ws/app/scripts ./scripts
COPY --from=web-builder /ws/app/lib ./lib
COPY --from=web-builder /ws/app/public ./public
COPY --from=web-builder /ws/app/assets ./assets

# Full server tree (for the in-app installer's runtime Go-package builds). The
# generated go.work + package_extensions.go + go.mod/go.sum land here; the
# `replace => ../../core/server` and go.work `use ../../node_modules/...` paths
# resolve against the members + node_modules copied above.
COPY --from=go-builder /ws/app/server/ ./server/

# bundled-packages.json so core's coreserver.SyncBundledPackages can find it at
# startup. Generated at app/server/bundled-packages.json; the binary reads it
# relative to its own dir, so place a copy at /app for the boot-time seed.
COPY --from=web-builder /ws/app/server/bundled-packages.json ./bundled-packages.json

# Data dir (PB writes /app/pb_data relative to cwd) + the generated-types dir.
# coreserver.DefaultTypesDir() resolves to <binaryDir>/../../core/types — with
# the binary at /app/tinycld that is /core/types — where the schema hook writes
# pbSchema.ts / pbZodSchema.ts at boot. Create it so the write succeeds.
RUN mkdir -p /app/pb_data /core/types

# Runtime-only bin scripts. The full bin/ directory in the app member includes
# dev helpers (debug-ios-boot, server) we deliberately don't ship.
COPY app/bin/prune-releases.sh ./bin/prune-releases.sh
RUN chmod +x ./bin/*.sh

COPY app/config/dokku.app.json ./app.json
COPY app/config/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

# Hand the entire app tree AND the workspace-root state to the tinycld user.
# pb_data/ and types/ are bind-mount targets whose mount-time ownership is the
# host's responsibility. The chown spans / one level up too, because the
# runtime scripts and in-app installer write under the workspace root
# (node_modules, generated files).
#
# Must run BEFORE setcap below — chown strips file capabilities (it resets the
# security.capability xattr along with ownership), so setcap'ing first then
# chown'ing would silently wipe the cap.
RUN chown -R tinycld:tinycld /app \
    && chown -R tinycld:tinycld /node_modules /core /contacts /mail /calendar /drive /calc /text /google-takeout-import \
    && chown tinycld:tinycld /package.json /package-lock.json /.npmrc /tinycld.packages.ts

# Grant cap_net_bind_service so the non-root user can bind :80/:443 when
# autocert is on (SERVE_ON_DOMAINS set). The plain-HTTP path defaults to the
# unprivileged :7090 and needs no special permissions.
#
# Caveat: the in-app package installer rebuilds the binary with `go build` and
# os.Renames it into place. The new binary has no caps. On autocert hosts that
# use the installer, the operator needs to re-apply the cap manually (the image
# ships setcap for this) or restart from the original image. Plain HTTP is fine.
RUN setcap 'cap_net_bind_service=+ep' ./tinycld

# 7090: plain HTTP (default, when SERVE_ON_DOMAINS is unset)
# 80:   autocert HTTP-01 challenge + plain-HTTP redirect (when SERVE_ON_DOMAINS is set)
# 443:  autocert HTTPS (when SERVE_ON_DOMAINS is set)
# 993:  IMAPS (implicit TLS)
# 465:  SMTPS (implicit TLS)
EXPOSE 7090 80 443 993 465

USER tinycld

# Container runs entirely as uid 1000 (tinycld). The binary's
# cap_net_bind_service file capability lets it bind :80/:443 when autocert is
# enabled; the plain-HTTP default of :7090 is unprivileged.
#
# When SERVE_ON_DOMAINS is set (space-separated), serve with autocert on those
# domains (binds :80 + :443 directly, terminates TLS in-process):
#   dokku config:set myapp SERVE_ON_DOMAINS="tinycld.com tinycld.org www.tinycld.org"
# Otherwise serve plain HTTP on :7090 (override with HTTP_ADDR), expecting an
# upstream reverse proxy or compose port mapping to route to it.
ENTRYPOINT ["./entrypoint.sh"]
