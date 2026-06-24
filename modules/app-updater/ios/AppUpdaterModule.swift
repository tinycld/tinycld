import CryptoKit
import ExpoModulesCore
import Foundation

public class AppUpdaterModule: Module {
    public func definition() -> ModuleDefinition {
        Name("AppUpdaterModule")

        Function("getEmbeddedId") { embeddedId() }
        Function("getRuntimeVersion") { embeddedRuntimeVersion() }
        Function("getCurrentBundleId") { Store.shared.currentId() ?? embeddedId() }
        Function("getCurrentBundleHash") { Store.shared.currentBundleHash(embeddedId: embeddedId()) }

        AsyncFunction("stageBundle") { (localDir: String, id: String, hash: String) in
            try Store.shared.stagePending(dir: localDir, id: id, hash: hash)
        }

        Function("markBundleHealthy") { Store.shared.markHealthy() }

        // Persist a crash-detail string (e.g. the offending regex pattern + the
        // Hermes error) next to the rollback markers, so the rolled-back NEXT
        // launch can upload it via report-bad. The JS-layer Sentry event often
        // can't flush before the process aborts; this on-disk record survives.
        Function("recordBundleError") { (detail: String) in Store.shared.recordError(detail) }

        // Mark the active OTA bundle bad so the NEXT bundleURL() reverts to the
        // previous bundle. Called from the JS global fatal handler when a
        // not-yet-healthy bundle throws fatally — paired with reload() it recovers
        // in-session instead of waiting for the crash-launch counter to trip.
        Function("markBundleBad") { Store.shared.markBad() }

        // Read-once: returns { id, hash } of a bundle rolled back since the last
        // call (or nil). JS reports it to the server's report-bad endpoint on boot.
        Function("takeRevertedBundle") { Store.shared.takeRevertedBundle() }

        AsyncFunction("reload") {
            DispatchQueue.main.async { Store.shared.requestReload() }
        }
    }

    private func embeddedId() -> String {
        Bundle.main.object(forInfoDictionaryKey: "TinyCldBundleId") as? String ?? "embedded"
    }
    private func embeddedRuntimeVersion() -> String {
        Bundle.main.object(forInfoDictionaryKey: "TinyCldRuntimeVersion") as? String ?? ""
    }
}

final class Store {
    static let shared = Store()
    private let fm = FileManager.default

    private var root: URL {
        let base = fm.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("app-updater", isDirectory: true)
        try? fm.createDirectory(at: base, withIntermediateDirectories: true)
        return base
    }
    private var currentURL: URL { root.appendingPathComponent("current.json") }
    private var pendingURL: URL { root.appendingPathComponent("pending.json") }
    private var previousURL: URL { root.appendingPathComponent("previous.json") }
    private var bootURL: URL { root.appendingPathComponent("boot.json") }
    private var revertURL: URL { root.appendingPathComponent("revert.json") }
    private var revertedURL: URL { root.appendingPathComponent("reverted.json") }
    private var embeddedHashURL: URL { root.appendingPathComponent("embedded-hash.json") }
    private var errorURL: URL { root.appendingPathComponent("error.json") }

    // Crash-launch rollback thresholds: a promoted OTA bundle is rolled back after
    // this many launches that never reach the JS "healthy" signal.
    //
    // A freshly-promoted bundle is on TRIAL until its first markHealthy: it rolls
    // back after the 2nd un-healthy launch (i.e. ONE visible crash, then recovery
    // on the next launch). This is the "roll back on first crash" backstop behind
    // the JS fatal handler's in-session revert.
    //
    // A bundle that was ALREADY healthy at least once keeps the more generous
    // threshold so a normal boot that's force-quit before markHealthy (slow first
    // paint) isn't mis-read as a crash and rolled back.
    private let rollbackAfterTrialLaunches = 2
    private let rollbackAfterLaunches = 3

    /// Reads a string field from current.json, treating an empty string as nil —
    /// matching the Android store's `optString(...).ifEmpty { null }`, so an empty
    /// pointer field is uniformly "absent" across platforms.
    private func currentString(_ key: String) -> String? {
        guard let v = readJSON(currentURL)?[key] as? String, !v.isEmpty else { return nil }
        return v
    }
    func currentId() -> String? { currentString("id") }
    private func currentDir() -> String? { currentString("dir") }
    private func currentHash() -> String? { currentString("hash") }

    func stagePending(dir: String, id: String, hash: String) throws {
        writeJSON(pendingURL, ["id": id, "dir": dir, "hash": hash])
    }

    /// Marks the active OTA bundle healthy so crash-rollback won't revert it.
    /// Called from JS after the app reaches a stable state — the earlier it runs,
    /// the smaller the window in which a healthy bundle could be mis-rolled-back.
    /// Removing boot.json also clears the trial flag, so a later post-healthy crash
    /// falls under the generous (non-trial) threshold rather than the trial one.
    /// Also drop any stale recorded crash detail — once a bundle is healthy, a
    /// prior fatal's detail must not attach to a future unrelated rollback.
    func markHealthy() {
        try? fm.removeItem(at: bootURL)
        try? fm.removeItem(at: errorURL)
    }

    /// Flags the active OTA bundle for revert on the next bundleURL(). The JS
    /// global fatal handler calls this (then reload()) when a not-yet-healthy
    /// bundle throws fatally, so recovery happens within the same reload cycle
    /// instead of waiting for the crash-launch counter. Records the current id so
    /// promotePendingIfAny only honors a revert that still targets that bundle.
    func markBad() {
        guard let id = currentId() else { return }
        writeJSON(revertURL, ["id": id])
    }

    /// Records a crash-detail string (the offending regex pattern + Hermes error,
    /// built by the JS fatal handler) for the active bundle, to be uploaded on the
    /// next (rolled-back) launch. Overwrites any prior record — the most recent
    /// fatal is the relevant one. Best-effort; a write failure is non-fatal.
    func recordError(_ detail: String) {
        writeJSON(errorURL, ["detail": detail, "id": currentId() ?? ""])
    }

    /// Returns `[id, hash, error]` of a bundle that was rolled back since the last
    /// call, then clears it (read-once). The recovered bundle calls this on boot
    /// and, if non-nil, POSTs report-bad so the server stops advertising the bad
    /// bundle to the fleet. `error` carries the persisted crash detail (e.g. the
    /// regex pattern that aborted Hermes) when one was recorded, else "". Returns
    /// nil when nothing was rolled back.
    func takeRevertedBundle() -> [String: String]? {
        guard let r = readJSON(revertedURL) else { return nil }
        try? fm.removeItem(at: revertedURL)
        let id = r["id"] as? String ?? ""
        if id.isEmpty { return nil }
        // Drain the recorded crash detail (read-once, paired with the revert).
        let detail = (readJSON(errorURL)?["detail"] as? String) ?? ""
        try? fm.removeItem(at: errorURL)
        return ["id": id, "hash": r["hash"] as? String ?? "", "error": detail]
    }

    /// Hex SHA-256 of the bundle the app is currently running. For a promoted OTA
    /// bundle this is the hash recorded at stage time (matches the server's
    /// bundle_hash). With no OTA bundle promoted it's the embedded bundle's hash,
    /// computed once and cached keyed by the embedded id so a later app version
    /// (new embedded id) re-hashes. Returns "" if the embedded bundle can't be
    /// found/hashed — the server tolerates an empty hash (falls back to id check).
    func currentBundleHash(embeddedId: String) -> String {
        if let h = currentHash() { return h }
        return embeddedBundleHash(embeddedId: embeddedId)
    }

    private func embeddedBundleHash(embeddedId: String) -> String {
        if let cached = readJSON(embeddedHashURL),
            cached["id"] as? String == embeddedId,
            let h = cached["hash"] as? String {
            return h
        }
        guard let url = Bundle.main.url(forResource: "main", withExtension: "jsbundle"),
            let data = try? Data(contentsOf: url) else {
            return ""
        }
        let hex = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
        writeJSON(embeddedHashURL, ["id": embeddedId, "hash": hex])
        return hex
    }

    /// Streaming lowercase-hex SHA-256 of the file at `path`, read in 64KB chunks
    /// so a large .hbc never lands in memory all at once. Returns nil if the file
    /// can't be opened/read — the caller treats nil as a verification failure
    /// (rollback), which is the safe direction.
    private func sha256HexOfFile(at path: String) -> String? {
        guard let handle = FileHandle(forReadingAtPath: path) else { return nil }
        defer { try? handle.close() }
        var hasher = SHA256()
        while true {
            let chunk: Data
            do {
                guard let next = try handle.read(upToCount: 1 << 16) else { break }
                chunk = next
            } catch {
                return nil
            }
            if chunk.isEmpty { break }
            hasher.update(data: chunk)
        }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    /// Called from the AppDelegate's `bundleURL()` BEFORE the React bridge loads.
    /// Promotes a pending bundle, applies crash-rollback, and returns the `.hbc`
    /// to load — or `nil` to fall back to the embedded `main.jsbundle`.
    func resolveBundleURL() -> URL? {
        promotePendingIfAny()
        // A JS fatal handler may have flagged the current bundle bad (markBad) just
        // before triggering this reload. Honor it FIRST — if the flag still targets
        // the current bundle, revert to previous before doing anything else, so an
        // in-session recovery never even loads the bad bundle again. A stale flag
        // (id no longer current) is cleared and ignored.
        if let revertId = readJSON(revertURL)?["id"] as? String {
            try? fm.removeItem(at: revertURL)
            if revertId == currentId() {
                rollbackToPrevious()
                return resolveAfterRollback()
            }
        }
        guard let dir = currentDir(), let id = currentId() else { return nil }
        guard let bundlePath = locateHbc(in: dir) else {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        // Defense-in-depth behind the JS-side SHA-256 verify (which runs once,
        // pre-stage). The staged bundle lives in a writable app dir and isn't
        // re-checked until the next cold launch, so re-hash the .hbc here and
        // refuse to load it if it no longer matches the hash recorded at stage
        // time. This catches post-stage tampering / corruption — treat a mismatch
        // exactly like a missing bundle and roll back rather than load unverified
        // bytes. (current.json `hash` == the server's bundle_hash == sha256 of
        // this .hbc; see downloadAndStage + app_native_export.go sha256OfFile.)
        if let want = currentHash(), !want.isEmpty,
            sha256HexOfFile(at: bundlePath)?.caseInsensitiveCompare(want) != .orderedSame {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        var boot = readJSON(bootURL) ?? ["id": id, "launchCount": 0, "trial": true]
        if (boot["id"] as? String) != id { boot = ["id": id, "launchCount": 0, "trial": true] }
        let count = (boot["launchCount"] as? Int ?? 0) + 1
        // A bundle that has never been marked healthy is on trial → roll back after
        // one un-healthy crash; an already-healthy bundle keeps the generous margin.
        let threshold = (boot["trial"] as? Bool ?? false) ? rollbackAfterTrialLaunches : rollbackAfterLaunches
        if count >= threshold {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        boot["launchCount"] = count
        writeJSON(bootURL, boot)
        return URL(fileURLWithPath: bundlePath)
    }

    private func promotePendingIfAny() {
        guard let p = readJSON(pendingURL),
            let pendingId = p["id"] as? String, p["dir"] is String else { return }
        // Idempotency guard for the crash window. This runs before the React bridge
        // loads, so the OS can kill us mid-promote. The three writes below are not
        // atomic as a set and `pending` is removed LAST — without this guard, a kill
        // after `current` is written but before `pending` is removed would, on the
        // next boot, re-run promote and overwrite `previous` (the good rollback
        // target) with the bundle we just promoted, destroying the safety net. If
        // the pending id already equals current, the promote already completed: just
        // clear the leftover pending pointer and return.
        if pendingId == (readJSON(currentURL)?["id"] as? String) {
            try? fm.removeItem(at: pendingURL)
            return
        }
        if let cur = readJSON(currentURL) { writeJSON(previousURL, cur) }
        writeJSON(currentURL, p)
        try? fm.removeItem(at: pendingURL)
        // Fresh promote: reset the crash tracker AS A TRIAL (rolls back after one
        // un-healthy crash) and clear any stale revert flag from a prior bundle.
        writeJSON(bootURL, ["id": p["id"] as? String ?? "", "launchCount": 0, "trial": true])
        try? fm.removeItem(at: revertURL)
    }

    private func rollbackToPrevious() {
        // Record the bundle being rolled back (id + hash) so the recovered bundle
        // can report it bad to the server on the next boot — takeRevertedBundle().
        // The crash handler can't report at crash time (the process is dying), so
        // we stash it and let the good bundle POST it once it's safely running.
        if let cur = readJSON(currentURL) {
            writeJSON(revertedURL, ["id": cur["id"] as? String ?? "", "hash": cur["hash"] as? String ?? ""])
        }
        if let prev = readJSON(previousURL) { writeJSON(currentURL, prev) }
        else { try? fm.removeItem(at: currentURL) }
        try? fm.removeItem(at: previousURL)
        try? fm.removeItem(at: bootURL)
        try? fm.removeItem(at: revertURL)
    }
    private func resolveAfterRollback() -> URL? {
        guard let dir = currentDir() else { return nil }
        return locateHbc(in: dir).map { URL(fileURLWithPath: $0) }
    }

    func requestReload() {
        AppUpdaterReload.trigger(withReason: "app-updater OTA reload")
    }

    private func locateHbc(in dir: String) -> String? {
        // `dir` is a filesystem path. JS stages from expo-file-system, whose
        // documentDirectory is a `file://` URI; URL(fileURLWithPath:) would treat
        // that prefix as a literal path component and never find the .hbc
        // (silently rolling back to embedded). Resolve a file:// URI properly,
        // otherwise treat the string as a plain path.
        let dirURL = dir.hasPrefix("file://")
            ? (URL(string: dir) ?? URL(fileURLWithPath: dir))
            : URL(fileURLWithPath: dir)
        let base = dirURL.appendingPathComponent("native/ios", isDirectory: true)
        guard let en = fm.enumerator(at: base, includingPropertiesForKeys: nil) else { return nil }
        for case let f as URL in en where f.pathExtension == "hbc" { return f.path }
        return nil
    }
    private func readJSON(_ u: URL) -> [String: Any]? {
        guard let d = try? Data(contentsOf: u) else { return nil }
        return (try? JSONSerialization.jsonObject(with: d)) as? [String: Any]
    }
    private func writeJSON(_ u: URL, _ v: [String: Any]) {
        if let d = try? JSONSerialization.data(withJSONObject: v) { try? d.write(to: u, options: .atomic) }
    }
}
