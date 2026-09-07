package org.tinycld.editorwebview

import expo.modules.kotlin.functions.Queues
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * `editor-webview`: a WebView host whose page survives React remounting the
 * host elsewhere. See src/EditorWebView.types.ts for the contract.
 */
class EditorWebViewModule : Module() {
  override fun definition() = ModuleDefinition {
    Name("EditorWebView")

    // A JS reload tears the module down; the pages would otherwise outlive the
    // hooks that own them. The Activity going away matters too: the pooled
    // WebViews were created with it as their context.
    OnDestroy { EditorWebViewPool.destroyAll() }
    OnActivityDestroys { EditorWebViewPool.destroyAll() }

    // Synchronous so the hosting hook keeps its `post(): boolean` contract — the
    // return value is "an instance exists", and the WebView call itself is
    // posted to the main thread inside.
    Function("postMessage") { instanceKey: String, data: String ->
      EditorWebViewPool.postMessage(instanceKey, data)
    }

    AsyncFunction("requestFocus") { instanceKey: String ->
      EditorWebViewPool.requestFocus(instanceKey)
    }.runOnQueue(Queues.MAIN)

    AsyncFunction("destroy") { instanceKey: String ->
      EditorWebViewPool.destroy(instanceKey)
    }.runOnQueue(Queues.MAIN)

    Function("getState") { instanceKey: String ->
      EditorWebViewPool.state(instanceKey)
    }

    View(EditorWebViewHost::class) {
      Events("onMessage", "onLoad", "onProcessGone")

      Prop("instanceKey") { view: EditorWebViewHost, key: String ->
        view.instanceKey = key
      }

      Prop("source") { view: EditorWebViewHost, html: String ->
        view.source = html
      }

      Prop("scrollEnabled") { view: EditorWebViewHost, enabled: Boolean ->
        view.scrollEnabled = enabled
      }

      Prop("webBackgroundColor") { view: EditorWebViewHost, color: Int? ->
        view.webBackgroundColor = color
      }

      Prop("inspectable") { view: EditorWebViewHost, enabled: Boolean ->
        view.inspectable = enabled
      }

      OnViewDidUpdateProps { view: EditorWebViewHost ->
        view.syncAttach()
      }

      OnViewDestroys { view: EditorWebViewHost ->
        view.detach()
      }
    }
  }
}
