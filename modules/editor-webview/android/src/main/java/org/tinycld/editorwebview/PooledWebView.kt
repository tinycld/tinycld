package org.tinycld.editorwebview

import android.annotation.SuppressLint
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.view.ViewGroup
import android.webkit.JavascriptInterface
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient

/**
 * A WebView configured for the editor page. Settings follow
 * react-native-webview's defaults for an `{ html }` source, minus everything a
 * self-contained editor page has no use for (file access, multiple windows,
 * zoom controls, downloads).
 */
@SuppressLint("SetJavaScriptEnabled", "ViewConstructor")
class PooledWebView(context: Context, val instanceKey: String) : WebView(context) {
  companion object {
    /** The global the page posts through; the same name react-native-webview installs. */
    const val BRIDGE_NAME = "ReactNativeWebView"
  }

  private var scrollLocked = false

  init {
    settings.javaScriptEnabled = true
    settings.domStorageEnabled = true
    settings.allowFileAccess = false
    settings.allowContentAccess = false
    settings.mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
    settings.setSupportMultipleWindows(false)
    settings.builtInZoomControls = false
    settings.displayZoomControls = false
    layoutParams = ViewGroup.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.MATCH_PARENT
    )

    // Unlike iOS no shim script is needed: the Java object IS
    // window.ReactNativeWebView, present before any page script runs.
    addJavascriptInterface(Bridge(instanceKey), BRIDGE_NAME)

    webViewClient = object : WebViewClient() {
      override fun onPageFinished(view: WebView, url: String) {
        EditorWebViewPool.onPageFinished(this@PooledWebView)
      }

      // The page is the whole product; a link inside it must not navigate the
      // editor away. Only the source's own load passes.
      override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean =
        request.url.toString() != EditorWebViewPool.PAGE_URL

      override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean =
        EditorWebViewPool.onRenderProcessGone(this@PooledWebView)
    }
  }

  /**
   * With scrolling off the WebView grows with its content inside an outer
   * ScrollView and must not consume the scroll gesture. react-native-webview
   * has no Android counterpart to iOS's scrollEnabled, so this is what the
   * editor's `scrollEnabled={false}` finally means here.
   */
  fun setScrollLocked(locked: Boolean) {
    scrollLocked = locked
    overScrollMode = if (locked) OVER_SCROLL_NEVER else OVER_SCROLL_IF_CONTENT_SCROLLS
    isVerticalScrollBarEnabled = !locked
    isHorizontalScrollBarEnabled = !locked
  }

  override fun overScrollBy(
    deltaX: Int,
    deltaY: Int,
    scrollX: Int,
    scrollY: Int,
    scrollRangeX: Int,
    scrollRangeY: Int,
    maxOverScrollX: Int,
    maxOverScrollY: Int,
    isTouchEvent: Boolean
  ): Boolean {
    if (scrollLocked) return false
    return super.overScrollBy(
      deltaX, deltaY, scrollX, scrollY, scrollRangeX, scrollRangeY, maxOverScrollX, maxOverScrollY, isTouchEvent
    )
  }

  override fun scrollTo(x: Int, y: Int) {
    if (scrollLocked) return
    super.scrollTo(x, y)
  }

  private class Bridge(private val key: String) {
    private val mainHandler = Handler(Looper.getMainLooper())

    /** Called from the page's JS thread whenever it runs window.ReactNativeWebView.postMessage. */
    @JavascriptInterface
    fun postMessage(message: String) {
      mainHandler.post { EditorWebViewPool.deliver(key, message) }
    }
  }
}
