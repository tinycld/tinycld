import ExpoModulesCore
import os.log
import WebKit

/// One pooled editor page: a live WKWebView that outlives every host it is
/// shown in.
final class EditorWebViewEntry {
  let webView: WKWebView
  let source: String
  var isLoaded = false
  /// The host currently showing the page, if any. Weak: the host is a React
  /// view with its own lifetime; the pool must never keep one alive.
  weak var host: EditorWebViewHostView?

  init(webView: WKWebView, source: String) {
    self.webView = webView
    self.source = source
  }
}

/// Forwards script messages without retaining the pool. The user content
/// controller retains its handlers, so a strong handler → pool → web view →
/// controller chain would be a cycle; react-native-webview's
/// `RNCWeakScriptMessageDelegate` exists for the same reason.
final class ScriptMessageProxy: NSObject, WKScriptMessageHandler {
  private let key: String
  private weak var pool: EditorWebViewPool?

  init(key: String, pool: EditorWebViewPool) {
    self.key = key
    self.pool = pool
  }

  func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
    guard message.name == EditorWebViewPool.messageHandlerName, let body = message.body as? String else {
      return
    }
    pool?.deliver(key, body)
  }
}

/// The pool: WebViews keyed by instance, attached to whichever host is
/// mounted. Everything here runs on the main thread except `postMessage` and
/// `state`, which JS calls synchronously and which only read the table under
/// the lock before hopping to main.
///
/// Messages from a page reach JS as MODULE events (`emit`), never as events on
/// the host view: a host is a React view that comes and goes with every
/// hand-off, and an event dispatched to a view React Native is tearing down at
/// that moment is dropped. The page's `editor-mounted` after a hand-off was
/// lost exactly that way on Android.
final class EditorWebViewPool: NSObject, WKNavigationDelegate {
  static let shared = EditorWebViewPool()
  static let messageHandlerName = "ReactNativeWebView"
  /// The page's origin. Not about:blank: a document with an opaque origin has
  /// every uncaught error masked to "Script error." by WebKit, which turned a
  /// page crash during a hand-off into an empty box with no message anywhere.
  /// Nothing is ever fetched from this host; it exists to give the page an
  /// origin of its own.
  static let pageURL = URL(string: "https://editor.tinycld.invalid/")!
  /// `log stream --predicate 'subsystem == "org.tinycld.editorwebview"'` shows the
  /// pool's lifecycle on a device: creation, attach, load, process death.
  static let log = OSLog(subsystem: "org.tinycld.editorwebview", category: "pool")

  /// Set by the module once it exists; the pool has no module reference of its own.
  var emit: ((String, [String: Any]) -> Void)?

  private var entries: [String: EditorWebViewEntry] = [:]
  private let lock = NSLock()

  private func entry(_ key: String) -> EditorWebViewEntry? {
    lock.lock()
    defer { lock.unlock() }
    return entries[key]
  }

  private func store(_ key: String, _ entry: EditorWebViewEntry?) {
    lock.lock()
    defer { lock.unlock() }
    entries[key] = entry
  }

  private func takeAll() -> [EditorWebViewEntry] {
    lock.lock()
    defer { lock.unlock() }
    let all = Array(entries.values)
    entries.removeAll()
    return all
  }

  private func find(_ webView: WKWebView) -> (String, EditorWebViewEntry)? {
    lock.lock()
    defer { lock.unlock() }
    return entries.first { $0.value.webView === webView }.map { ($0.key, $0.value) }
  }

  // MARK: - Attach / detach (main thread)

  /// Show the page for `key` in `host`, creating and loading it on first use.
  /// Last mount wins: whichever host attaches most recently takes the page.
  func attach(_ key: String, host: EditorWebViewHostView, source: String) {
    let existing = self.entry(key)
    let entry = existing ?? create(key, source: source)
    // The one line that proves a hand-off did not reload: "reused" means the
    // page kept everything it had.
    os_log(.debug, log: Self.log, "attach %{public}@: %{public}@", key, existing == nil ? "created" : "reused")
    if entry.source != source {
      os_log(.error, log: Self.log, "ignoring a different source for %{public}@; the source is fixed at creation", key)
    }
    entry.host = host
    if entry.webView.superview !== host {
      // addSubview moves the view out of its previous superview.
      host.addSubview(entry.webView)
      entry.webView.frame = host.bounds
    }
    host.apply(to: entry.webView)
  }

  /// Drop the host's claim. Guarded on identity so the order in which React
  /// commits a hand-off (old host removed before or after the new one is added)
  /// cannot clear the new host's binding. Deliberately does nothing to the
  /// web view: destroying it here is exactly what react-native-webview does
  /// and what this pool exists to avoid.
  func detach(_ key: String, host: EditorWebViewHostView) {
    guard let entry = entry(key), entry.host === host else {
      return
    }
    entry.host = nil
  }

  func webView(for key: String, ownedBy host: EditorWebViewHostView) -> WKWebView? {
    guard let entry = entry(key), entry.host === host else {
      return nil
    }
    return entry.webView
  }

  // MARK: - Messages

  /// Page → JS. Main thread (WKScriptMessageHandler delivers there).
  func deliver(_ key: String, _ data: String) {
    emit?("onMessage", ["instanceKey": key, "data": data])
  }

  /// JS → page. Any thread; returns whether an instance exists to receive it.
  /// Dispatched on `document`, the one target both platforms use — the page
  /// listens on `window` too, and dispatching on both would deliver twice.
  func postMessage(_ key: String, _ data: String) -> Bool {
    guard let entry = entry(key) else {
      return false
    }
    guard let json = try? JSONSerialization.data(withJSONObject: ["data": data]),
      let literal = String(data: json, encoding: .utf8) else {
      return false
    }
    let script = "document.dispatchEvent(new MessageEvent('message', \(literal)));"
    DispatchQueue.main.async {
      entry.webView.evaluateJavaScript(script, completionHandler: nil)
    }
    return true
  }

  func requestFocus(_ key: String) {
    entry(key)?.webView.becomeFirstResponder()
  }

  func state(_ key: String) -> [String: Any] {
    let entry = self.entry(key)
    return [
      "exists": entry != nil,
      "loaded": entry?.isLoaded ?? false,
      "attached": entry?.host != nil
    ]
  }

  // MARK: - Lifecycle

  func destroy(_ key: String) {
    guard let entry = entry(key) else {
      return
    }
    os_log(.debug, log: Self.log, "destroy %{public}@", key)
    store(key, nil)
    teardown(entry)
  }

  func destroyAll() {
    for entry in takeAll() {
      teardown(entry)
    }
  }

  private func teardown(_ entry: EditorWebViewEntry) {
    let controller = entry.webView.configuration.userContentController
    controller.removeScriptMessageHandler(forName: Self.messageHandlerName)
    controller.removeAllUserScripts()
    entry.webView.navigationDelegate = nil
    entry.webView.removeFromSuperview()
    entry.host = nil
  }

  private func create(_ key: String, source: String) -> EditorWebViewEntry {
    let config = WKWebViewConfiguration()

    let controller = WKUserContentController()
    // The same global react-native-webview installs, so the page is unchanged:
    // it keeps calling window.ReactNativeWebView.postMessage(String).
    let shim = WKUserScript(
      source: """
      window.\(Self.messageHandlerName) = window.\(Self.messageHandlerName) || {};
      window.\(Self.messageHandlerName).postMessage = function (data) {
        window.webkit.messageHandlers.\(Self.messageHandlerName).postMessage(String(data));
      };
      """,
      injectionTime: .atDocumentStart,
      forMainFrameOnly: true
    )
    controller.addUserScript(shim)
    controller.add(ScriptMessageProxy(key: key, pool: self), name: Self.messageHandlerName)
    config.userContentController = controller
    config.allowsInlineMediaPlayback = true
    // An editor must not turn typed phone numbers and addresses into links.
    config.dataDetectorTypes = []

    let webView = WKWebView(frame: .zero, configuration: config)
    webView.navigationDelegate = self
    webView.allowsLinkPreview = false
    webView.allowsBackForwardNavigationGestures = false
    webView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
    webView.scrollView.contentInsetAdjustmentBehavior = .never
    EditorWebViewKeyboardHacks.hideInputAccessoryView(webView)
    EditorWebViewKeyboardHacks.disableKeyboardDisplayRequiresUserAction(webView)

    let entry = EditorWebViewEntry(webView: webView, source: source)
    store(key, entry)
    webView.loadHTMLString(source, baseURL: Self.pageURL)
    return entry
  }

  // MARK: - WKNavigationDelegate

  func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
    guard let found = find(webView) else {
      return
    }
    let (key, entry) = found
    entry.isLoaded = true
    os_log(.debug, log: Self.log, "loaded %{public}@", key)
    emit?("onLoad", ["instanceKey": key])
  }

  /// The content process died (memory pressure, a WebKit crash). Reload the
  /// same source; the page posts `editor-ready` again as it boots and the JS
  /// host re-sends init for the new page epoch.
  func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
    guard let found = find(webView) else {
      return
    }
    let (key, entry) = found
    entry.isLoaded = false
    os_log(.error, log: Self.log, "content process terminated %{public}@; reloading", key)
    webView.loadHTMLString(entry.source, baseURL: Self.pageURL)
    emit?("onProcessGone", ["instanceKey": key])
  }

  func webView(
    _ webView: WKWebView,
    decidePolicyFor navigationAction: WKNavigationAction,
    decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
  ) {
    // The page is the whole product; a link inside it must not navigate the
    // editor away. Only the initial (and any reload) load of the source passes.
    let isOwnLoad = navigationAction.request.url == Self.pageURL
    decisionHandler(isOwnLoad ? .allow : .cancel)
  }
}
