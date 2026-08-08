module tinycld.org/core

go 1.26.3

// Build against the multi-org PocketBase fork, vendored at
// tinycld/third_party/pocketbase: core's jsvm registration uses the fork-only
// jsvm.Config.OnInit to install its $-bindings, so core's standalone build/test
// must resolve the fork too. Same module path + base tag (v0.39.8) as upstream —
// no version skew. Drop this once the fork seams are upstreamed. The app
// (tinycld/server) carries the matching replace for the assembled build.
replace github.com/pocketbase/pocketbase => ../../third_party/pocketbase

require (
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/SherClockHolmes/webpush-go v1.4.0
	github.com/coder/websocket v1.8.14
	github.com/disintegration/imaging v1.6.2
	github.com/emersion/go-ical v0.0.0-20240127095438-fc1c9d8fb2b6
	github.com/emersion/go-imap/v2 v2.0.0-beta.8
	github.com/emersion/go-message v0.18.2
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.24.0
	github.com/emersion/go-vcard v0.0.0-20260618161152-d854b7e0e2d3
	github.com/emersion/go-webdav v0.7.0
	github.com/getsentry/sentry-go v0.44.1
	github.com/google/uuid v1.6.0
	github.com/grafana/sobek v0.0.0-20260722203707-64fef69693b6
	github.com/mrz1836/postmark v1.9.0
	github.com/nathanstitt/doctaculous v0.1.0
	github.com/pocketbase/dbx v1.12.0
	github.com/pocketbase/pocketbase v0.39.8
	github.com/skyterra/y-crdt v0.0.0-20260224023949-c0cb10d3f33e
	github.com/spf13/cobra v1.10.2
	github.com/yuin/goldmark v1.8.2
	golang.org/x/crypto v0.54.0
	golang.org/x/image v0.44.0
	golang.org/x/net v0.57.0
	modernc.org/sqlite v1.54.0
)

require (
	github.com/adrg/strutil v0.2.2 // indirect
	github.com/adrg/sysfont v0.1.2 // indirect
	github.com/adrg/xdg v0.3.0 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beevik/etree v1.6.0 // indirect
	github.com/benoitkugler/pstokenizer v1.0.1 // indirect
	github.com/benoitkugler/textlayout v0.3.2 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/domodwyer/mailyak/v3 v3.6.2 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/evanw/esbuild v0.28.1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/ganigeorgiev/fexpr v0.5.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/pprof v0.0.0-20260604005048-7023385849c0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.23 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pocketbase/ozzo-validation/v4 v4.3.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
