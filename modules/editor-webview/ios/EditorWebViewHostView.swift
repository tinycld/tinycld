import ExpoModulesCore
import WebKit

/// The React-side host: a plain view the pooled WebView is placed into while
/// this host is on screen. It owns nothing — attach on entering a window,
/// release the claim on leaving one, keep the WebView sized to its bounds.
/// Messages from the page do not pass through it (see EditorWebViewPool).
final class EditorWebViewHostView: ExpoView {
  var instanceKey = "" {
    didSet {
      if oldValue != instanceKey, !oldValue.isEmpty {
        EditorWebViewPool.shared.detach(oldValue, host: self)
      }
    }
  }
  var source: String?
  var scrollEnabled = true
  var webBackgroundColor: UIColor?
  var inspectable = false

  required init(appContext: AppContext? = nil) {
    super.init(appContext: appContext)
    clipsToBounds = true
  }

  /// Attach only from a window. Fabric may create and configure a host before
  /// mounting it; taking the page then would pull it off the screen into a
  /// view nobody can see.
  func syncAttach() {
    guard window != nil, !instanceKey.isEmpty, let source else {
      return
    }
    EditorWebViewPool.shared.attach(instanceKey, host: self, source: source)
  }

  override func didMoveToWindow() {
    super.didMoveToWindow()
    if window != nil {
      syncAttach()
    }
  }

  override func willMove(toSuperview newSuperview: UIView?) {
    super.willMove(toSuperview: newSuperview)
    if newSuperview == nil, !instanceKey.isEmpty {
      EditorWebViewPool.shared.detach(instanceKey, host: self)
    }
  }

  override func layoutSubviews() {
    super.layoutSubviews()
    EditorWebViewPool.shared.webView(for: instanceKey, ownedBy: self)?.frame = bounds
  }

  /// Per-instance settings that travel with the host: whoever holds the page
  /// decides how it scrolls and what sits behind it.
  func apply(to webView: WKWebView) {
    webView.scrollView.isScrollEnabled = scrollEnabled
    if let color = webBackgroundColor {
      let opaque = color.cgColor.alpha == 1.0
      webView.isOpaque = opaque
      webView.scrollView.backgroundColor = color
      webView.backgroundColor = color
    }
    if #available(iOS 16.4, *) {
      webView.isInspectable = inspectable
    }
  }
}
