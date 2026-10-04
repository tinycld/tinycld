module tinycld.org/core

go 1.27.1

// Build against the PocketBase fork, vendored at
// tinycld/third_party/pocketbase: core's jsvm registration uses the fork-only
// jsvm.Config.OnInit to install its $-bindings, so core's standalone build/test
// must resolve the fork too. Same module path + base tag (v0.40.4) as upstream —
// no version skew. Drop this once the fork seams are upstreamed. The app
// (tinycld/server) carries the matching replace for the assembled build.
replace github.com/pocketbase/pocketbase => ../../third_party/pocketbase

// The archive format is a nested module so the CLI can read backup archives
// without depending on core (or the PocketBase fork).
replace tinycld.org/core/backup/format => ./backup/format

require (
	filippo.io/age v1.3.2
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/SherClockHolmes/webpush-go v1.4.0
	github.com/coder/websocket v1.8.14
	github.com/disintegration/imaging v1.6.2
	github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608
	github.com/emersion/go-imap/v2 v2.0.0-beta.8
	github.com/emersion/go-message v0.18.2
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.24.0
	github.com/emersion/go-vcard v0.0.0-20260618161152-d854b7e0e2d3
	github.com/emersion/go-webdav v0.7.0
	github.com/ganigeorgiev/fexpr v0.6.0
	github.com/getsentry/sentry-go v0.44.1
	github.com/google/uuid v1.6.0
	github.com/grafana/sobek v0.0.0-20260915160442-8a431c44cd7b
	github.com/klauspost/compress v1.20.0
	github.com/mrz1836/postmark v1.9.0
	github.com/nathanstitt/omnidoc v1.0.0
	github.com/osshield/gopbs v0.0.0-00010101000000-000000000000
	github.com/pocketbase/dbx v1.12.0
	github.com/pocketbase/ozzo-validation/v4 v4.3.0
	github.com/pocketbase/pocketbase v0.40.4
	github.com/skyterra/y-crdt v0.0.0-20260224023949-c0cb10d3f33e
	github.com/spf13/cobra v1.10.2
	github.com/yuin/goldmark v1.8.2
	golang.org/x/crypto v0.57.0
	golang.org/x/image v0.46.0
	golang.org/x/net v0.59.0
	golang.org/x/text v0.42.0
	modernc.org/sqlite v1.57.0
	tinycld.org/core/backup/format v0.0.0
)

require (
	filippo.io/hpke v0.4.0 // indirect
	github.com/HugoSmits86/nativewebp v1.3.0 // indirect
	github.com/adrg/strutil v0.2.2 // indirect
	github.com/adrg/sysfont v0.1.2 // indirect
	github.com/adrg/xdg v0.3.0 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beevik/etree v1.6.0 // indirect
	github.com/benoitkugler/pstokenizer v1.0.1 // indirect
	github.com/benoitkugler/textlayout v0.3.2 // indirect
	github.com/dlclark/regexp2/v2 v2.8.0 // indirect
	github.com/domodwyer/mailyak/v3 v3.6.2 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/pprof v0.0.0-20260902005441-ca85771921e4 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.0 // indirect
)

replace github.com/osshield/gopbs => github.com/nathanstitt/gopbs v1.0.0

// go-webdav's CalDAV PropPatch fails the whole request with 501, which macOS
// Calendar reads as the collection not supporting PROPPATCH — it writes
// displayname/calendar-color/calendar-order while adopting a calendar, so on
// the hard 501 it drops the calendar and the account shows up empty. The fork
// answers per-property inside a 207 like carddav already does. Drop this once
// https://github.com/emersion/go-webdav/pull/216 lands in a release.
replace github.com/emersion/go-webdav => github.com/nathanstitt/go-webdav v0.7.1-0.20261001184608-67abd707e045
