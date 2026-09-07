package org.tinycld.editorwebview

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.Color
import android.webkit.WebView
import expo.modules.kotlin.AppContext
import expo.modules.kotlin.records.Field
import expo.modules.kotlin.records.Record
import expo.modules.kotlin.viewevent.EventDispatcher
import expo.modules.kotlin.views.ExpoView

data class MessageEvent(@Field val data: String) : Record

data class InstanceEvent(@Field val instanceKey: String) : Record

/**
 * The React-side host: a plain view group the pooled WebView is placed into
 * while this host is on screen. It owns nothing — attach on entering the
 * window, release the claim on leaving it.
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

  // React Native coalesces same-named view events that share a coalescing key
  // and land in one frame, keeping only the last. The page posts bursts —
  // `editor-mounted` immediately followed by `content-height` — and losing the
  // first of those leaves the host waiting forever, so every message gets its
  // own key.
  private var messageSequence = 0
  val onMessage by EventDispatcher<MessageEvent> { (messageSequence++ and 0x7fff).toShort() }
  val onLoad by EventDispatcher<InstanceEvent>()
  val onProcessGone by EventDispatcher<InstanceEvent>()

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
