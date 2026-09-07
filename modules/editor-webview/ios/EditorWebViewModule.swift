import ExpoModulesCore

/// `editor-webview`: a WebView host whose page survives React remounting the
/// host elsewhere. See src/EditorWebView.types.ts for the contract.
public final class EditorWebViewModule: Module {
  public func definition() -> ModuleDefinition {
    Name("EditorWebView")

    // A JS reload tears the module down; the pages would otherwise outlive the
    // hooks that own them.
    OnDestroy {
      DispatchQueue.main.async {
        EditorWebViewPool.shared.destroyAll()
      }
    }

    // Synchronous so the hosting hook keeps its `post(): boolean` contract — the
    // return value is "an instance exists", and the WebKit call itself is
    // dispatched to the main thread inside.
    Function("postMessage") { (instanceKey: String, data: String) -> Bool in
      EditorWebViewPool.shared.postMessage(instanceKey, data)
    }

    AsyncFunction("requestFocus") { (instanceKey: String) in
      EditorWebViewPool.shared.requestFocus(instanceKey)
    }.runOnQueue(.main)

    AsyncFunction("destroy") { (instanceKey: String) in
      EditorWebViewPool.shared.destroy(instanceKey)
    }.runOnQueue(.main)

    Function("getState") { (instanceKey: String) -> [String: Any] in
      EditorWebViewPool.shared.state(instanceKey)
    }

    View(EditorWebViewHostView.self) {
      Events("onMessage", "onLoad", "onProcessGone")

      Prop("instanceKey") { (view: EditorWebViewHostView, key: String) in
        view.instanceKey = key
      }

      Prop("source") { (view: EditorWebViewHostView, html: String) in
        view.source = html
      }

      Prop("scrollEnabled") { (view: EditorWebViewHostView, enabled: Bool) in
        view.scrollEnabled = enabled
      }

      Prop("webBackgroundColor") { (view: EditorWebViewHostView, color: UIColor?) in
        view.webBackgroundColor = color
      }

      Prop("inspectable") { (view: EditorWebViewHostView, enabled: Bool) in
        view.inspectable = enabled
      }

      OnViewDidUpdateProps { (view: EditorWebViewHostView) in
        view.syncAttach()
      }
    }
  }
}
