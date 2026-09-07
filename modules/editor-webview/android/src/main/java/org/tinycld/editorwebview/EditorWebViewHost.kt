package org.tinycld.editorwebview

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.Color
import android.webkit.WebView
import expo.modules.kotlin.AppContext
import expo.modules.kotlin.views.ExpoView

/**
 * The React-side host: a plain view group the pooled WebView is placed into
 * while this host is on screen. It owns nothing — attach on entering the
 * window, release the claim on leaving it. Messages from the page do not pass
 * through it (see EditorWebViewPool).
 */
@SuppressLint("ViewConstructor")
class EditorWebViewHost(context: Context, appContext: AppContext) : ExpoView(context, appContext) {
  var instanceKey = ""
    set(value) {
      if (field.isNotEmpty() && field != value) {
        EditorWebViewPool.detach(field, this)
      }
      field = value
    }
  var source: String? = null
  var scrollEnabled = true
  var webBackgroundColor: Color? = null
  var inspectable = false

  /**
   * Attach only from the window. React Native may create and configure a host
   * before adding it to the hierarchy; taking the page then would pull it off
   * the screen into a view nobody can see.
   */
  fun syncAttach() {
    if (!isAttachedToWindow || instanceKey.isEmpty()) return
    val html = source ?: return
    EditorWebViewPool.attach(instanceKey, this, html)
  }

  fun detach() {
    if (instanceKey.isNotEmpty()) {
      EditorWebViewPool.detach(instanceKey, this)
    }
  }

  override fun onAttachedToWindow() {
    super.onAttachedToWindow()
    syncAttach()
  }

  override fun onDetachedFromWindow() {
    detach()
    super.onDetachedFromWindow()
  }

  /**
   * Per-instance settings that travel with the host: whoever holds the page
   * decides how it scrolls and what sits behind it.
   */
  fun apply(webView: PooledWebView) {
    webView.setScrollLocked(!scrollEnabled)
    webBackgroundColor?.let { webView.setBackgroundColor(it.toArgb()) }
    WebView.setWebContentsDebuggingEnabled(inspectable)
  }
}
