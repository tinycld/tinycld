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
    private var embeddedHashURL: URL { root.appendingPathComponent("embedded-hash.json") }

    // A promoted OTA bundle is only rolled back after this many consecutive boots
    // that never reach the JS "healthy" signal. 3 (i.e. two fully-failed boots)
    // leaves margin for a boot that renders but is force-quit before markHealthy
    // fires, which a threshold of 2 would mis-read as a crash and roll back a
    // healthy bundle.
    private let rollbackAfterLaunches = 3

    func currentId() -> String? { readJSON(currentURL)?["id"] as? String }
    private func currentDir() -> String? { readJSON(currentURL)?["dir"] as? String }
    private func currentHash() -> String? { readJSON(currentURL)?["hash"] as? String }

    func stagePending(dir: String, id: String, hash: String) throws {
        writeJSON(pendingURL, ["id": id, "dir": dir, "hash": hash])
    }

    /// Marks the active OTA bundle healthy so crash-rollback won't revert it.
    /// Called from JS after the app reaches a stable state — the earlier it runs,
    /// the smaller the window in which a healthy bundle could be mis-rolled-back.
    func markHealthy() { try? fm.removeItem(at: bootURL) }

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

    /// Called from the AppDelegate's `bundleURL()` BEFORE the React bridge loads.
    /// Promotes a pending bundle, applies crash-rollback, and returns the `.hbc`
    /// to load — or `nil` to fall back to the embedded `main.jsbundle`.
    func resolveBundleURL() -> URL? {
        promotePendingIfAny()
        guard let dir = currentDir(), let id = currentId() else { return nil }
        guard let bundlePath = locateHbc(in: dir) else {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        var boot = readJSON(bootURL) ?? ["id": id, "launchCount": 0]
        if (boot["id"] as? String) != id { boot = ["id": id, "launchCount": 0] }
        let count = (boot["launchCount"] as? Int ?? 0) + 1
        if count >= rollbackAfterLaunches {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        boot["launchCount"] = count
        writeJSON(bootURL, boot)
        return URL(fileURLWithPath: bundlePath)
    }

    private func promotePendingIfAny() {
        guard let p = readJSON(pendingURL), p["id"] is String, p["dir"] is String else { return }
        if let cur = readJSON(currentURL) { writeJSON(previousURL, cur) }
        writeJSON(currentURL, p)
        try? fm.removeItem(at: pendingURL)
        try? fm.removeItem(at: bootURL)
    }

    private func rollbackToPrevious() {
        if let prev = readJSON(previousURL) { writeJSON(currentURL, prev) }
        else { try? fm.removeItem(at: currentURL) }
        try? fm.removeItem(at: previousURL)
        try? fm.removeItem(at: bootURL)
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
