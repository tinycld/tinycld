# Android (Google Play) Release Runbook

This document is the single source of truth for TinyCld's Google Play listing content, Data
safety declarations, app-access (review) credentials, and Android versioning policy. Update it
every time the Play Console listing changes. It is the Android counterpart to
[`ios-release.md`](./ios-release.md); where a field maps to its Apple equivalent, that is noted.

The native build itself is almost entirely shared with iOS — the same `app.json`, the same
`with-app-updater` config plugin (which already patches `MainApplication.kt` + `strings.xml`),
and the same `app-updater` native module (which has full Kotlin parity). This runbook therefore
covers **release wiring and store submission**, not native code.

## Play Console app record

- **App name:** TinyCld
- **Package name:** org.tinycld.app  *(immutable once the first AAB is uploaded — same id as iOS)*
- **Default category:** Productivity
- **Content rating:** completed via the IARC questionnaire in Play Console *(Apple "Age rating" equivalent)* — declare no objectionable content; expected outcome "Everyone".
- **Contact email:** support@tinycld.org
- **Website:** https://tinycld.org
- **Privacy Policy URL:** https://tinycld.org/privacy  *(required)*
- **Copyright:** © 2026 Nathan Stitt

## Store listing copy

Play splits the description into two fields (and, unlike the App Store, has **no keyword field** —
discovery is ASO-driven from the description text, so weave the iOS keywords into the prose).

- **Short description** (≤ 80 chars):

      Self-hosted mail, calendar, contacts & drive on a server you control.

- **Full description** (≤ 4000 chars) — reuse the iOS description:

      TinyCld is a self-hosted productivity suite that connects to a PocketBase server you
      control. Your mail, calendar, contacts, and files live on your own hardware — not ours.

      • Bring your own server. Point the app at your TinyCld instance to get going. Don't have
        one yet? Use the built-in demo server to try before you install.
      • Mail. Threaded conversations, labels, attachments, IMAP/SMTP.
      • Calendar. Recurring events, guest RSVP, reminders, CalDAV.
      • Contacts. Shared directory, favorites, CardDAV.
      • Drive. Files, versioning, share links, role-based permissions, WebDAV.
      • Open source. AGPL-3.0. Every line is auditable at github.com/tinycld/tinycld.

      No analytics, no ad SDKs, no tracking pixels. Diagnostic crash reports are anonymized.

## Graphics inventory (Play-specific)

Play requires a different asset set from the App Store. All are uploaded in Play Console → Store
presence → Main store listing.

| Asset | Spec | Required |
| --- | --- | --- |
| App icon | 512 × 512 PNG, 32-bit with alpha | Yes |
| Feature graphic | 1024 × 500 PNG/JPG (no alpha) | Yes |
| Phone screenshots | 2–8, 16:9 or 9:16, min edge ≥ 320 px | Yes (min 2) |
| 7" tablet screenshots | up to 8 | Recommended |
| 10" tablet screenshots | up to 8 | Recommended (the app supports tablets) |

Capture the same five compositions used for iOS: (1) mail thread view, (2) calendar week view,
(3) contacts list with detail pane, (4) drive file browser, (5) settings with About section +
dark mode visible.

## Data safety form

Play Console → App content → Data safety *(Apple "App Privacy" equivalent)*. Declare exactly what
Sentry collects — it is the only third party:

- **Data collected:** Crash logs, Diagnostics.
- **Collected / shared:** Collected, not shared.
- **Linked to user identity:** No.
- **Used for tracking:** No.
- **Encrypted in transit:** Yes.
- **User can request deletion:** Yes (in-app Delete account; see below).
- **Purposes:** App functionality, Analytics.

The form must match actual SDK behavior — a mismatch is a common rejection cause.

## App access (review credentials)

Play Console → App content → App access. TinyCld is login-gated and self-hosted, so Google **will
reject it as a login wall** unless demo credentials are provided. Reuse the iOS App Review demo
account (`REVIEW_DEMO_EMAIL` / `REVIEW_DEMO_PASSWORD` in the workspace `.env`; the password lives
in 1Password, entry "App Review demo password"). Provide these instructions verbatim:

    All functionality requires sign-in. TinyCld is self-hosted: users normally connect the app
    to their own PocketBase server. For review, use the hosted demo server and pre-seeded account:

        Demo server URL: https://tinycld.org
        Email:           appreview@tinycld.org
        Password:        <FILL IN FROM 1PASSWORD>

    On the first screen, tap "Use tinycld.org" to connect to the demo server, then sign in with
    the credentials above. The demo account is reseeded nightly, so Mail, Calendar, Contacts, and
    Drive are always populated.

## Account deletion

Play Console → App content → Data deletion. Google requires a deletion path for any app with
accounts. Declare both:

- **In-app:** Settings → Personal → Delete my account — type the account email to confirm. This
  anonymizes the user record and removes the user from every org (same flow as iOS).
- **Public URL:** the web deletion-request page (confirm it is published before submitting).

## Version / versionCode policy

- **Marketing version** (`versionName`) lives in `app.json` `expo.version`. Bump it for every Play
  submission. `app.config.ts` injects it at build time.
- **`versionCode`** (the integer Play requires to increase monotonically) is auto-incremented by
  EAS (`production` profile, `autoIncrement: true`, with `appVersionSource: remote` so EAS tracks
  it server-side). The equivalent of the iOS `buildNumber`.
- **Never** hand-edit `android/app/build.gradle`. The `android/` directory is gitignored and
  regenerated by every `expo prebuild`.

## EAS build profiles (see `eas.json`)

| Profile | Artifact | Purpose |
| --- | --- | --- |
| `development` | APK (internal) | Local iteration / dev client |
| `preview` | APK (internal) | Team testing — sideload via `adb install` |
| `review` | APK (internal), `EXPO_PUBLIC_APP_REVIEW_MODE=1` | Exposes the "Fill demo credentials" button |
| `production` | **AAB** (`buildType: app-bundle`) | Google Play upload |

Profiles with `distribution: internal` build an APK (directly installable on a device); the
`production` profile builds an AAB (Play upload format — **not** directly installable; see the
verification section). **Never submit a `review` build to Play.**

## Local build prerequisites (this machine)

The Android scripts mirror iOS and build **locally** (`--local`). Two RN-0.83-on-this-machine
hazards are already handled so you don't have to think about them:

- **JDK 17 (handled by the script).** RN 0.83's native (CMake/NDK) build requires JDK 17; the
  machine default `java` is JDK 24, under which the native configure step fails with
  `WARNING: A restricted method in java.lang.System has been called`. The `build:android` script
  therefore self-pins `JAVA_HOME` to JDK 17 (`/usr/libexec/java_home -v 17`) for the EAS
  subprocess — you do **not** need to export anything. (Requires a JDK 17 installed; Zulu 17 is
  present. If absent, install Temurin/Zulu 17.) This is macOS-specific, like `build:ios`.
- **Non-symlinked build working dir (handled by the script) — the `libworklets.so` fix.**
  `react-native-reanimated` / `expo-modules-core` link `react-native-worklets`' `libworklets.so`
  by absolute path. On macOS `/tmp` is a symlink to `/private/tmp`, and `eas build --local`
  defaults its working dir to `/tmp/eas-build-local-nodejs/…`; CMake/Ninja then resolve the
  worklets `.so` under one path spelling and reference it under the other, so the link fails with
  `ninja: error: '.../libworklets.so' … missing and no known rule to make it` even though worklets
  built it and the tasks ran in the right order. (Reanimated issue #9151; expo/expo #42892/#42893.)
  The `build:android` script sets `EAS_LOCAL_BUILD_WORKINGDIR=$HOME/.cache/tinycld-eas-build` (a
  real, non-symlinked path) to sidestep it. Disabling Gradle parallelism / patching the native
  Gradle files does **not** help — the cause is the symlink, not task ordering. The same stack
  builds fine on **EAS cloud** (Linux, no `/tmp` symlink), which is the fallback if a local build
  ever regresses here.
- **Android SDK** — `ANDROID_HOME` must be set (it is: `~/Library/Android/sdk`), with NDK +
  build-tools + a platform installed (all present). If Gradle ever reports missing packages or
  unaccepted licenses, install `cmdline-tools` and run `sdkmanager --licenses`.
- **EAS auth** — `pnpm exec eas whoami`; log in if needed. The first build generates the Android
  **upload keystore** automatically (EAS stores it; Google holds the real app-signing key via Play
  App Signing) — no interactive prompt in the `--local` flow once you're authenticated.

## Submission steps

1. Ensure this file is up to date.
2. `pnpm run packages:generate` (picks up any new package wiring).
3. `pnpm run checks` — must pass.
4. `pnpm exec expo prebuild --platform android --clean` — regenerates `android/` from `app.json`
   (applies the `with-app-updater` OTA seam).
5. `pnpm run build:android` → `./tmp/build.aab`. The script self-pins JDK 17 and sets a
   non-symlinked `EAS_LOCAL_BUILD_WORKINGDIR` — no manual env setup. (~7–10 min from cold.)
6. **First release only — MANUAL upload.** Google does not allow the *first* AAB for a new package
   to be uploaded via the API. In Play Console: create the app, fill the listing + Data safety +
   App access forms from this document, and upload `./tmp/build.aab` to the **Internal testing**
   track by hand.
7. Run through the end-to-end checklist on a physical device (see below).
8. **Subsequent releases** can use the API: `pnpm run submit:android` (uploads `./tmp/build.aab`
   to the Internal testing track as a draft, per `eas.json` `submit.production.android`).
9. Promote Internal → Closed/Open testing → Production in the Play Console when ready. Releases
   stay manual (`releaseStatus: draft`) so post-upload issues can be caught first.

## End-to-end verification checklist

Build a `preview` APK for device testing (an AAB can't be `adb install`ed directly):

    pnpm exec eas build --platform android --profile preview --local --output ./tmp/preview.apk
    adb install ./tmp/preview.apk

- Fresh install → lands on `/connect` without AuthGate flash.
- "Use tinycld.org" (primary CTA) → connects directly to the demo server.
- "I host my own server" → opens the URL sheet → Connect succeeds with a custom address.
- Sign in with demo account → primary org loads.
- Browse mail, calendar, contacts, drive — all populated.
- Toggle system dark mode → app follows.
- Settings → Personal → About shows version, commit, server.
- Settings → Personal → Disconnect server → returns to `/connect`, credentials cleared.
- Sign back in → Delete account → type email → confirm → returns to `/connect` and old email can
  no longer sign in.
- **Deep link:** opening `https://tinycld.org/demo` launches the app directly (not a chooser).
  Requires `https://tinycld.org/.well-known/assetlinks.json` to be served with the **Play App
  Signing** SHA-256 fingerprint (copy it from Play Console → Setup → App signing). Until that file
  is published, `autoVerify` falls back to the chooser — a server-side prerequisite, not an app bug.
- Force a test crash → Sentry receives the event with scrubbed PII.
- **Push notification** from server → delivered on a real device. Requires FCM setup (below).

## Known Play rejection risks (document mitigations)

- **Login wall / self-hosted misread as broken.** Mitigation: App access demo credentials above +
  the "Use tinycld.org" default-server button eliminate friction.
- **Target API level floor.** Google enforces a minimum `targetSdkVersion` for new submissions
  (confirm the current floor in Play Console at submission time). Expo SDK 55 / compileSdk 35
  should satisfy it; verify the generated `android/app/build.gradle` `targetSdkVersion` after
  prebuild.
- **Data safety mismatch.** The form must match Sentry's actual behavior — keep it in sync.
- **Missing privacy policy / data-deletion URL.** Both are required for account apps; URLs above.
- **`RECORD_AUDIO` permission.** Declared in `app.json`; justify its use in the listing if Play
  flags it during review.

## FCM push setup (post-build follow-up)

Android push is **independent of the build and submission** — the build is fine without it, but
push silently won't work. `core/lib/expo-push.ts` calls `Notifications.getExpoPushTokenAsync()`,
which on Android resolves through Firebase Cloud Messaging (FCM V1). Without an FCM credential on
the Expo project, that call throws and `registerExpoPushToken` returns `false`.

1. Create (or link) a Firebase project for `org.tinycld.app`.
2. Firebase Console → Project settings → Cloud Messaging → ensure the **Firebase Cloud Messaging
   API (V1)** is enabled.
3. Project settings → Service accounts → generate a new private key → download the FCM V1
   service-account JSON.
4. Upload it to the Expo project credentials (**not** the repo):
   `pnpm exec eas credentials` → Android → "Push Notifications: Manage your FCM V1 Service Account
   Key", or via the EAS dashboard → Credentials → Android → FCM V1.
5. Verify on a real device: token registers, server sends a push, device receives it.

**No `google-services.json` and no `googleServicesFile` in `app.json` are required** — the app
uses Expo push tokens, not the native Firebase SDK. Keep `app.json` clean. (Android 13+ runtime
notification permission is requested by `expo-notifications`; no manual `POST_NOTIFICATIONS` entry
is needed.)

## Credentials & secrets

- **Upload keystore:** generated and stored by EAS on the first build (Play App Signing holds the
  real signing key). Not in the repo.
- **Play submit service account:** a Google Cloud service-account JSON with Play Console release
  permissions, saved at the **workspace root** as `google-play-service-account.json` and
  referenced by `eas.json` `submit.production.android.serviceAccountKeyPath`. It is gitignored
  (`.gitignore`) and excluded from build/image archives (`.easignore`, `.dockerignore`). Never
  commit it.
- **FCM V1 key:** uploaded to Expo credentials (above), not the repo.

## Assets

Store large binary listing assets (screenshots, feature graphic, marketing icon) outside this repo
(e.g. `~/Dropbox/tinycld/play-store/`). The repo tracks policy and configuration only.
