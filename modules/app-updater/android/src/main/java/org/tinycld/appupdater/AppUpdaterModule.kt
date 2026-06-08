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
    private val embeddedHashFile = File(root, "embedded-hash.json")

    // A promoted OTA bundle is only rolled back after this many consecutive boots
    // that never reach the JS "healthy" signal. 3 (i.e. two fully-failed boots)
    // leaves margin for a boot that renders but is force-quit before markHealthy
    // fires, which a threshold of 2 would mis-read as a crash and roll back a
    // healthy bundle.
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
        bootFile.delete()
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
     * Called from the RN host's `getJSBundleFile()` BEFORE the React instance
     * loads. Promotes a pending bundle, applies crash-rollback, and returns the
     * `.hbc` path to load — or `null` to fall back to the embedded bundle.
     */
    fun resolveBundlePath(): String? {
        promotePendingIfAny()
        val dir = currentDir() ?: return null
        val id = currentId() ?: return null
        val bundlePath = locateHbc(dir)
        if (bundlePath == null) {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        var boot = readJSON(bootFile)
        if (boot == null || boot.optString("id") != id) {
            boot = JSONObject().put("id", id).put("launchCount", 0)
        }
        val count = boot.optInt("launchCount", 0) + 1
        if (count >= rollbackAfterLaunches) {
            rollbackToPrevious()
            return resolveAfterRollback()
        }
        boot.put("launchCount", count)
        writeJSON(bootFile, boot)
        return bundlePath
    }

    private fun promotePendingIfAny() {
        val pending = readJSON(pendingFile) ?: return
        if (!pending.has("id") || !pending.has("dir")) return
        readJSON(currentFile)?.let { writeJSON(previousFile, it) }
        writeJSON(currentFile, pending)
        pendingFile.delete()
        bootFile.delete()
    }

    /**
     * Roll back to the previous bundle (or embedded if none). Deleting
     * previous.json is critical: without it a rollback target that ALSO crashes
     * twice would loop forever instead of falling through to embedded.
     */
    private fun rollbackToPrevious() {
        val prev = readJSON(previousFile)
        if (prev != null) writeJSON(currentFile, prev) else currentFile.delete()
        previousFile.delete()
        bootFile.delete()
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

    private fun locateHbc(dir: String): String? =
        File(File(dir), "native/android")
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
            file.writeText(value.toString())
        } catch (_: Exception) {
            // best-effort persistence; a failed write simply leaves prior state
        }
    }
}
