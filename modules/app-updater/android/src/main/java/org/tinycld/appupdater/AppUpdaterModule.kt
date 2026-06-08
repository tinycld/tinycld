package org.tinycld.appupdater

import android.content.Context
import com.facebook.react.ReactApplication
import com.facebook.react.bridge.UiThreadUtil
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import java.io.File
import org.json.JSONObject

class AppUpdaterModule : Module() {
    private val context: Context
        get() = appContext.reactContext ?: throw IllegalStateException("no react context")

    override fun definition() = ModuleDefinition {
        Name("AppUpdaterModule")

        Function("getEmbeddedId") { Store(context).embeddedId() }
        Function("getRuntimeVersion") { Store(context).embeddedRuntime() }
        Function("getCurrentBundleId") { Store(context).currentId() ?: Store(context).embeddedId() }

        AsyncFunction("stageBundle") { localDir: String, id: String ->
            Store(context).stagePending(localDir, id)
        }

        Function("markBundleHealthy") { Store(context).clearBootMarker() }

        AsyncFunction("reload") { Store(context).requestReload() }
    }
}

/**
 * File-backed pointer store mirroring the iOS `Store`. Maintains four JSON files
 * in an app-private dir and drives staging / promote / crash-rollback.
 *
 *   current.json   { id, dir }  — active OTA bundle (absent = run embedded)
 *   pending.json   { id, dir }  — staged, promoted on next launch
 *   previous.json  { id, dir }  — prior current, rollback target
 *   boot.json      { id, launchCount } — crash tracker
 */
class Store(private val context: Context) {
    private val root: File =
        File(context.filesDir, "app-updater").apply { if (!exists()) mkdirs() }

    private val currentFile = File(root, "current.json")
    private val pendingFile = File(root, "pending.json")
    private val previousFile = File(root, "previous.json")
    private val bootFile = File(root, "boot.json")

    fun embeddedId(): String = resString("tinycld_bundle_id") ?: "embedded"

    fun embeddedRuntime(): String = resString("tinycld_runtime_version") ?: ""

    fun currentId(): String? = readJSON(currentFile)?.optString("id")?.ifEmpty { null }

    private fun currentDir(): String? = readJSON(currentFile)?.optString("dir")?.ifEmpty { null }

    fun stagePending(dir: String, id: String) {
        writeJSON(pendingFile, JSONObject().put("id", id).put("dir", dir))
    }

    fun clearBootMarker() {
        bootFile.delete()
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
        if (count >= 2) {
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
