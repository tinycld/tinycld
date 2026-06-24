package org.tinycld.appupdater

import android.content.Context
import com.facebook.react.ReactApplication
import com.facebook.react.bridge.UiThreadUtil
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import java.io.File
import java.security.MessageDigest
import org.json.JSONObject

class AppUpdaterModule : Module() {
    private val context: Context
        get() = appContext.reactContext ?: throw IllegalStateException("no react context")

    override fun definition() = ModuleDefinition {
        Name("AppUpdaterModule")

        Function("getEmbeddedId") { Store(context).embeddedId() }
        Function("getRuntimeVersion") { Store(context).embeddedRuntime() }
        Function("getCurrentBundleId") { Store(context).currentId() ?: Store(context).embeddedId() }
        Function("getCurrentBundleHash") {
            val store = Store(context)
            store.currentBundleHash(store.embeddedId())
        }

        AsyncFunction("stageBundle") { localDir: String, id: String, hash: String ->
            Store(context).stagePending(localDir, id, hash)
        }

        Function("markBundleHealthy") { Store(context).markHealthy() }

        // Persist a crash-detail string (the offending regex pattern + Hermes
        // error) next to the rollback markers, so the rolled-back NEXT launch can
        // upload it via report-bad. The JS-layer Sentry event often can't flush
        // before the process aborts; this on-disk record survives. Mirrors iOS.
        Function("recordBundleError") { detail: String -> Store(context).recordError(detail) }

        // Mark the active OTA bundle bad so the next getJSBundleFile() reverts to
        // the previous bundle. Called from the JS global fatal handler (paired with
        // reload()) so a not-yet-healthy bundle that throws fatally recovers
        // in-session instead of waiting for the crash-launch counter. Mirrors iOS.
        Function("markBundleBad") { Store(context).markBad() }

        // Read-once: { id, hash } of a bundle rolled back since the last call (or
        // null). JS reports it to the server's report-bad endpoint on boot.
        Function("takeRevertedBundle") { Store(context).takeRevertedBundle() }

        AsyncFunction("reload") { Store(context).requestReload() }
    }
}

/**
 * File-backed pointer store mirroring the iOS `Store`. Maintains JSON pointer
 * files in an app-private dir and drives staging / promote / crash-rollback.
 *
 *   current.json        { id, dir, hash }  — active OTA bundle (absent = embedded)
 *   pending.json        { id, dir, hash }  — staged, promoted on next launch
 *   previous.json       { id, dir, hash }  — prior current, rollback target
 *   boot.json           { id, launchCount } — crash tracker
 *   embedded-hash.json  { id, hash }        — cached embedded-bundle hash
 */
class Store(private val context: Context) {
    private val root: File =
        File(context.filesDir, "app-updater").apply { if (!exists()) mkdirs() }

    private val currentFile = File(root, "current.json")
    private val pendingFile = File(root, "pending.json")
    private val previousFile = File(root, "previous.json")
    private val bootFile = File(root, "boot.json")
    private val revertFile = File(root, "revert.json")
    private val revertedFile = File(root, "reverted.json")
    private val embeddedHashFile = File(root, "embedded-hash.json")
    private val errorFile = File(root, "error.json")

    // Crash-launch rollback thresholds (mirrors iOS). A freshly-promoted bundle is
    // on TRIAL until its first markHealthy: it rolls back after the 2nd un-healthy
    // launch (ONE visible crash, then recovery) — the backstop behind the JS fatal
    // handler's in-session revert. A bundle that was already healthy keeps the more
    // generous threshold so a slow first paint that's force-quit before markHealthy
    // isn't mis-read as a crash.
    private val rollbackAfterTrialLaunches = 2
    private val rollbackAfterLaunches = 3

    fun embeddedId(): String = resString("tinycld_bundle_id") ?: "embedded"

    fun embeddedRuntime(): String = resString("tinycld_runtime_version") ?: ""

    fun currentId(): String? = readJSON(currentFile)?.optString("id")?.ifEmpty { null }

    private fun currentDir(): String? = readJSON(currentFile)?.optString("dir")?.ifEmpty { null }

    private fun currentHash(): String? = readJSON(currentFile)?.optString("hash")?.ifEmpty { null }

    fun stagePending(dir: String, id: String, hash: String) {
        writeJSON(pendingFile, JSONObject().put("id", id).put("dir", dir).put("hash", hash))
    }

    /**
     * Marks the active OTA bundle healthy so crash-rollback won't revert it.
     * Called from JS once the app reaches a stable state — the earlier it runs,
     * the smaller the window in which a healthy bundle could be mis-rolled-back.
     */
    fun markHealthy() {
        // Clearing boot.json also clears the trial flag, so a later post-healthy
        // crash falls under the generous (non-trial) threshold, not the trial one.
        bootFile.delete()
        // Drop any stale recorded crash detail — once a bundle is healthy, a prior
        // fatal's detail must not attach to a future unrelated rollback.
        errorFile.delete()
    }

    /**
     * Records a crash-detail string (the offending regex pattern + Hermes error,
     * built by the JS fatal handler) for the active bundle, to be uploaded on the
     * next (rolled-back) launch. Overwrites any prior record. Mirrors iOS.
     */
    fun recordError(detail: String) {
        writeJSON(errorFile, JSONObject().put("detail", detail).put("id", currentId() ?: ""))
    }

    /**
     * Flags the active OTA bundle for revert on the next getJSBundleFile(). The JS
     * global fatal handler calls this (then reload()) when a not-yet-healthy bundle
     * throws fatally, so recovery happens within the same reload cycle instead of
     * waiting for the crash-launch counter. Records the current id so promote only
     * honors a revert that still targets that bundle. Mirrors iOS markBad().
     */
    fun markBad() {
        val id = currentId() ?: return
        writeJSON(revertFile, JSONObject().put("id", id))
    }

    /**
     * Returns a map { id, hash } of a bundle rolled back since the last call, then
     * clears it (read-once). The recovered bundle calls this on boot and, if
     * non-null, POSTs report-bad so the server stops advertising the bad bundle to
     * the fleet. Returns null when nothing was rolled back. Mirrors iOS.
     */
    fun takeRevertedBundle(): Map<String, String>? {
        val r = readJSON(revertedFile) ?: return null
        revertedFile.delete()
        val id = r.optString("id").ifEmpty { return null }
        // Drain the recorded crash detail (read-once, paired with the revert).
        val detail = readJSON(errorFile)?.optString("detail") ?: ""
        errorFile.delete()
        return mapOf("id" to id, "hash" to r.optString("hash"), "error" to detail)
    }

    /**
     * Hex SHA-256 of the bundle the app is currently running. For a promoted OTA
     * bundle this is the hash recorded at stage time (matches the server's
     * bundle_hash). With no OTA bundle promoted it's the embedded bundle's hash,
     * computed once and cached keyed by the embedded id. Returns "" if the
     * embedded bundle can't be read — the server tolerates an empty hash.
     */
    fun currentBundleHash(embeddedId: String): String =
        currentHash() ?: embeddedBundleHash(embeddedId)

    private fun embeddedBundleHash(embeddedId: String): String {
        readJSON(embeddedHashFile)?.let { cached ->
            if (cached.optString("id") == embeddedId) {
                cached.optString("hash").ifEmpty { null }?.let { return it }
            }
        }
        return try {
            val digest = MessageDigest.getInstance("SHA-256")
            context.assets.open("index.android.bundle").use { input ->
                val buf = ByteArray(1 shl 16)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    digest.update(buf, 0, n)
                }
            }
            val hex = digest.digest().joinToString("") { "%02x".format(it) }
            writeJSON(embeddedHashFile, JSONObject().put("id", embeddedId).put("hash", hex))
            hex
        } catch (_: Exception) {
            ""
        }
    }

    /**
     * Streaming lowercase-hex SHA-256 of the file at [path], read in 64KB chunks
     * (mirrors the embedded-hash path) so a large .hbc never lands in memory all
     * at once. Returns null if the file can't be read — the caller treats null as
     * a verification failure (rollback), which is the safe direction.
     */
    private fun sha256HexOfFile(path: String): String? =
        try {
            val digest = MessageDigest.getInstance("SHA-256")
            File(path).inputStream().use { input ->
                val buf = ByteArray(1 shl 16)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    digest.update(buf, 0, n)
                }
            }
            digest.digest().joinToString("") { "%02x".format(it) }
        } catch (_: Exception) {
            null
        }

    /**
     * Called from the RN host's `getJSBundleFile()` BEFORE the React instance
     * loads. Promotes a pending bundle, applies crash-rollback, and returns the
     * `.hbc` path to load — or `null` to fall back to the embedded bundle.
     */
    fun resolveBundlePath(): String? {
        promotePendingIfAny()
        // A JS fatal handler may have flagged the current bundle bad (markBad) just
        // before triggering this reload. Honor it FIRST — if the flag still targets
        // the current bundle, revert before loading anything, so an in-session
        // recovery never reloads the bad bundle. A stale flag is cleared + ignored.
        readJSON(revertFile)?.optString("id")?.ifEmpty { null }?.let { revertId ->
            revertFile.delete()
            if (revertId == currentId()) {
                rollbackToPrevious("js-fatal markBad")
                return resolveAfterRollback()
            }
        }
        val dir = currentDir() ?: return null
        val id = currentId() ?: return null
        val bundlePath = locateHbc(dir)
        if (bundlePath == null) {
            rollbackToPrevious("staged .hbc missing")
            return resolveAfterRollback()
        }
        // Defense-in-depth behind the JS-side SHA-256 verify (which runs once,
        // pre-stage). The staged bundle lives in a writable app-private dir and
        // isn't re-checked until the next cold launch, so re-hash the .hbc here
        // and refuse to load it if it no longer matches the hash recorded at stage
        // time. This catches post-stage tampering / corruption — treat a mismatch
        // exactly like a missing bundle and roll back rather than load unverified
        // bytes. (current.json `hash` == the server's bundle_hash == sha256 of
        // this .hbc; see downloadAndStage + app_native_export.go sha256OfFile.)
        val want = currentHash()
        if (!want.isNullOrEmpty() && !sha256HexOfFile(bundlePath).equals(want, ignoreCase = true)) {
            rollbackToPrevious("hash mismatch")
            return resolveAfterRollback()
        }
        var boot = readJSON(bootFile)
        if (boot == null || boot.optString("id") != id) {
            boot = JSONObject().put("id", id).put("launchCount", 0).put("trial", true)
        }
        val count = boot.optInt("launchCount", 0) + 1
        // A bundle that has never been marked healthy is on trial → roll back after
        // one un-healthy crash; an already-healthy bundle keeps the generous margin.
        val threshold = if (boot.optBoolean("trial", false)) rollbackAfterTrialLaunches else rollbackAfterLaunches
        if (count >= threshold) {
            rollbackToPrevious("crash-launch counter tripped")
            return resolveAfterRollback()
        }
        boot.put("launchCount", count)
        writeJSON(bootFile, boot)
        return bundlePath
    }

    private fun promotePendingIfAny() {
        val pending = readJSON(pendingFile) ?: return
        if (!pending.has("id") || !pending.has("dir")) return
        val pendingId = pending.optString("id")
        // Idempotency guard for the crash window — see the iOS module for the full
        // rationale. This runs before the bridge loads; a kill after `current` is
        // written but before `pending` is deleted would otherwise re-run promote on
        // the next boot and clobber `previous` (the rollback target). If pending
        // already equals current, the promote completed: clear pending and return.
        if (pendingId == readJSON(currentFile)?.optString("id")) {
            pendingFile.delete()
            return
        }
        readJSON(currentFile)?.let { writeJSON(previousFile, it) }
        writeJSON(currentFile, pending)
        pendingFile.delete()
        // Fresh promote: reset the crash tracker AS A TRIAL (rolls back after one
        // un-healthy crash) and clear any stale revert flag from a prior bundle.
        writeJSON(bootFile, JSONObject().put("id", pending.optString("id")).put("launchCount", 0).put("trial", true))
        revertFile.delete()
    }

    /**
     * Roll back to the previous bundle (or embedded if none). Deleting
     * previous.json is critical: without it a rollback target that ALSO crashes
     * twice would loop forever instead of falling through to embedded.
     */
    /**
     * Rolls the active OTA bundle back to the previous one. `reason` names which
     * path fired so the report-bad upload always carries WHY — most rollbacks
     * happen here at native boot, BEFORE any JS runs, so without this the detail
     * field is empty and the server only learns "a bundle rolled back". Mirrors iOS.
     */
    private fun rollbackToPrevious(reason: String) {
        // Record the rolled-back bundle (id + hash) so the recovered bundle can
        // report it bad to the server on the next boot — see takeRevertedBundle().
        readJSON(currentFile)?.let {
            val curId = it.optString("id")
            writeJSON(revertedFile, JSONObject().put("id", curId).put("hash", it.optString("hash")))
            // Ensure error.json carries a detail for the upload. Prefer a richer
            // JS-supplied detail already recorded for THIS bundle (regex pattern +
            // Hermes message); otherwise stamp the native rollback reason.
            val existing = readJSON(errorFile)
            val jsDetail = existing?.optString("detail") ?: ""
            val jsId = existing?.optString("id") ?: ""
            if (jsDetail.isEmpty() || (jsId.isNotEmpty() && jsId != curId)) {
                val launches = readJSON(bootFile)?.optInt("launchCount") ?: 0
                writeJSON(
                    errorFile,
                    JSONObject()
                        .put("detail", "native rollback: $reason (launches=$launches)")
                        .put("id", curId)
                )
            }
        }
        val prev = readJSON(previousFile)
        if (prev != null) writeJSON(currentFile, prev) else currentFile.delete()
        previousFile.delete()
        bootFile.delete()
        revertFile.delete()
    }

    private fun resolveAfterRollback(): String? {
        val dir = currentDir() ?: return null
        return locateHbc(dir)
    }

    fun requestReload() {
        UiThreadUtil.runOnUiThread {
            val app = context.applicationContext as? ReactApplication
            app?.reactHost?.reload("app-updater OTA reload")
        }
    }

    // `dir` is a filesystem path. JS stages from expo-file-system, whose
    // documentDirectory is a `file://` URI; strip the scheme defensively so a
    // file:// prefix is never treated as a literal path component (which would
    // make the .hbc unfindable and silently roll back to embedded).
    private fun locateHbc(dir: String): String? =
        File(File(dir.removePrefix("file://")), "native/android")
            .walkTopDown()
            .firstOrNull { it.isFile && it.extension == "hbc" }
            ?.path

    private fun resString(name: String): String? {
        val id = context.resources.getIdentifier(name, "string", context.packageName)
        if (id == 0) return null
        return context.getString(id).ifEmpty { null }
    }

    private fun readJSON(file: File): JSONObject? {
        if (!file.exists()) return null
        return try {
            JSONObject(file.readText())
        } catch (_: Exception) {
            null
        }
    }

    private fun writeJSON(file: File, value: JSONObject) {
        try {
            // Write to a temp file then atomically rename, so a crash/kill
            // mid-write can't leave a truncated pointer file (which readJSON would
            // see as corrupt → lose the active/rollback pointer). Mirrors the iOS
            // store's `.atomic` write. rename within the same dir is atomic.
            val tmp = File(file.parentFile, "${file.name}.tmp")
            tmp.writeText(value.toString())
            if (!tmp.renameTo(file)) {
                // Fallback if rename fails (e.g. existing target on some FS): a
                // direct write is still better than dropping the update entirely.
                file.writeText(value.toString())
                tmp.delete()
            }
        } catch (_: Exception) {
            // best-effort persistence; a failed write simply leaves prior state
        }
    }
}
