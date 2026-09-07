package org.tinycld.editorwebview

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.ViewGroup
import android.view.inputmethod.InputMethodManager
import org.json.JSONObject

private const val TAG = "EditorWebView"
private const val BUFFER_LIMIT = 256

/**
 * The pool: WebViews keyed by instance, attached to whichever host is mounted.
 * Everything here runs on the main thread except [postMessage] and [state],
 * which JS calls synchronously and which only read the table under the lock
 * before posting to main.
 */
object EditorWebViewPool {
  /**
   * The page's origin. Not about:blank: a document with an opaque origin has
   * every uncaught error masked to "Script error.", which turned a page crash
   * during a hand-off into an empty box with no message anywhere. Nothing is
   * ever fetched from this host; it exists to give the page an origin of its own.
   */
  const val PAGE_URL = "https://editor.tinycld.invalid/"

  /** One pooled editor page: a live WebView that outlives every host it is shown in. */
  class Entry(var webView: PooledWebView, val source: String) {
    var isLoaded = false
    /** The host currently showing the page, if any. */
    var host: EditorWebViewHost? = null
    /**
     * Messages the page posted while no host was attached — replayed, in order,
     * on the next attach. Without this a page that boots while parentless would
     * post its one `editor-ready` into nothing, and the host would never send init.
     */
    val buffer = ArrayDeque<String>()
  }

  private val entries = HashMap<String, Entry>()
  private val mainHandler = Handler(Looper.getMainLooper())

  @Synchronized
  private fun entry(key: String): Entry? = entries[key]

  @Synchronized
  private fun store(key: String, entry: Entry?) {
    if (entry == null) entries.remove(key) else entries[key] = entry
  }

  @Synchronized
  private fun takeAll(): List<Entry> {
    val all = entries.values.toList()
    entries.clear()
    return all
  }

  @Synchronized
  private fun find(webView: PooledWebView): Pair<String, Entry>? =
    entries.entries.firstOrNull { it.value.webView === webView }?.let { it.key to it.value }

  // region Attach / detach (main thread)

  /**
   * Show the page for [key] in [host], creating and loading it on first use.
   * Last mount wins: whichever host attaches most recently takes the page.
   */
  fun attach(key: String, host: EditorWebViewHost, source: String) {
    val existing = entry(key)
    val entry = existing ?: create(key, webViewContext(host), source)
    // The one line that proves a hand-off did not reload: "reused" means the
    // page kept everything it had. `adb logcat -s EditorWebView`.
    Log.d(TAG, "attach $key: ${if (existing == null) "created" else "reused"}")
    if (entry.source != source) {
      Log.w(TAG, "ignoring a different source for instance $key; the source is fixed at creation")
    }
    entry.host = host
    val webView = entry.webView
    if (webView.parent !== host) {
      (webView.parent as? ViewGroup)?.removeView(webView)
      host.addView(
        webView,
        ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
      )
      // React Native ignores the requestLayout a late addView triggers, so the
      // child would stay 0×0 until the next RN layout pass.
      host.measureAndLayout()
    }
    host.apply(webView)

    while (entry.buffer.isNotEmpty()) {
      val data = entry.buffer.removeFirst()
      Log.d(TAG, "replay $key ${data.take(70)}")
      host.onMessage(MessageEvent(data))
    }
  }

  /**
   * Drop the host's claim. Guarded on identity so the order in which React
   * commits a hand-off cannot clear the new host's binding. Removes the WebView
   * from a dropped host's view group but never destroys it — destroying is
   * exactly what react-native-webview does and what this pool exists to avoid.
   */
  fun detach(key: String, host: EditorWebViewHost) {
    val entry = entry(key) ?: return
    if (entry.host !== host) return
    entry.host = null
    if (entry.webView.parent === host) {
      host.removeView(entry.webView)
    }
  }

  // endregion

  // region Messages

  /** Page → host. Main thread. */
  fun deliver(key: String, data: String) {
    val entry = entry(key) ?: return
    val host = entry.host
    Log.d(TAG, "deliver $key host=${host != null} attached=${host?.isAttachedToWindow} ${data.take(70)}")
    if (host != null) {
      host.onMessage(MessageEvent(data))
      return
    }
    entry.buffer.addLast(data)
    if (entry.buffer.size > BUFFER_LIMIT) {
      entry.buffer.removeFirst()
    }
  }

  /**
   * Host → page. Any thread; returns whether an instance exists to receive it.
   * The dispatch script is react-native-webview's, which already targets
   * `document` — the one target both platforms use.
   */
  fun postMessage(key: String, data: String): Boolean {
    val entry = entry(key) ?: return false
    val eventInit = JSONObject().put("data", data).toString()
    val script = "(function () {" +
      "var data = " + eventInit + ";" +
      "var event = new MessageEvent('message', data);" +
      "document.dispatchEvent(event);" +
      "})();"
    mainHandler.post { entry.webView.evaluateJavascript(script, null) }
    return true
  }

  fun requestFocus(key: String) {
    val webView = entry(key)?.webView ?: return
    webView.requestFocus()
    // react-native-webview stops at requestFocus; after a re-parent that alone
    // does not reliably bring the keyboard up.
    val imm = webView.context.getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager
    imm?.showSoftInput(webView, InputMethodManager.SHOW_IMPLICIT)
  }

  fun state(key: String): Map<String, Any> {
    val entry = entry(key)
    return mapOf(
      "exists" to (entry != null),
      "loaded" to (entry?.isLoaded ?: false),
      "attached" to (entry?.host != null)
    )
  }

  // endregion

  // region Lifecycle

  fun destroy(key: String) {
    val entry = entry(key) ?: return
    Log.d(TAG, "destroy $key")
    store(key, null)
    teardown(entry)
  }

  fun destroyAll() {
    takeAll().forEach { teardown(it) }
  }

  private fun teardown(entry: Entry) {
    entry.host = null
    (entry.webView.parent as? ViewGroup)?.removeView(entry.webView)
    entry.webView.removeJavascriptInterface(PooledWebView.BRIDGE_NAME)
    entry.webView.destroy()
  }

  /**
   * The Activity when there is one: it is what gives the WebView its IME and
   * text-selection action mode, the same context react-native-webview's
   * themed context resolves to.
   */
  private fun webViewContext(host: EditorWebViewHost): Context =
    host.appContext.currentActivity ?: host.context

  private fun create(key: String, context: Context, source: String): Entry {
    val webView = PooledWebView(context, key)
    val entry = Entry(webView, source)
    store(key, entry)
    webView.loadDataWithBaseURL(PAGE_URL, source, "text/html", "utf-8", null)
    return entry
  }

  /** From the WebView client, main thread. */
  fun onPageFinished(webView: PooledWebView) {
    val (key, entry) = find(webView) ?: return
    entry.isLoaded = true
    Log.d(TAG, "loaded $key")
    entry.host?.onLoad(InstanceEvent(key))
  }

  /**
   * The render process died. By contract the WebView instance is unusable
   * afterwards, so unlike iOS this is a rebuild: a fresh WebView with the same
   * source takes the dead one's place in the same host. The page posts
   * `editor-ready` again as it boots and the JS host re-sends init for the new
   * page epoch. Returns true because returning false would kill the app.
   */
  fun onRenderProcessGone(webView: PooledWebView): Boolean {
    val (key, entry) = find(webView) ?: return true
    Log.e(TAG, "render process gone for $key; rebuilding")
    val host = entry.host
    (webView.parent as? ViewGroup)?.removeView(webView)
    webView.removeJavascriptInterface(PooledWebView.BRIDGE_NAME)
    webView.destroy()

    val replacement = PooledWebView(webView.context, key)
    entry.webView = replacement
    entry.isLoaded = false
    replacement.loadDataWithBaseURL(PAGE_URL, entry.source, "text/html", "utf-8", null)
    if (host != null) {
      host.addView(
        replacement,
        ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
      )
      host.measureAndLayout()
      host.apply(replacement)
      host.onProcessGone(InstanceEvent(key))
    }
    return true
  }

  // endregion
}
