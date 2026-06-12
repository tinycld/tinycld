package org.tinycld.appupdater

import android.content.Context

/**
 * Public bridge over [Store] so the (config-plugin-injected) RN host
 * `getJSBundleFile()` override can resolve a staged OTA bundle path without
 * reaching into the module's internals. Compiled into the AppUpdater library
 * alongside [AppUpdaterModule], so `MainApplication` can reference it by its
 * fully-qualified name `org.tinycld.appupdater.AppUpdaterBundle`.
 *
 * Thin wrapper — no behavior of its own; delegates to [Store.resolveBundlePath].
 */
object AppUpdaterBundle {
    /**
     * Returns the staged bundle file path to load, or `null` to fall back to the
     * embedded `index.android.bundle`. Safe to call once, early, from
     * `getJSBundleFile()`.
     */
    @JvmStatic
    fun stagedBundlePath(context: Context): String? = Store(context).resolveBundlePath()
}
