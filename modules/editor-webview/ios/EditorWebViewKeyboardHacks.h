#import <Foundation/Foundation.h>
#import <WebKit/WebKit.h>

NS_ASSUME_NONNULL_BEGIN

/// The two keyboard behaviours react-native-webview obtains through the ObjC
/// runtime, ported (MIT) for a WKWebView we own. Kept in ObjC because both are
/// IMP-level surgery on WebKit's private WKContentView, and a verbatim port is
/// the lowest-risk way to ship exactly what react-native-webview already ships
/// in this binary.
@interface EditorWebViewKeyboardHacks : NSObject

/// Drops the accessory bar (Done, undo/redo, iPad shortcuts) above the keyboard
/// for this web view. Per instance: it re-classes the web view's content view.
+ (void)hideInputAccessoryView:(WKWebView *)webView;

/// Lets a programmatic `focus()` inside the page open the keyboard, which WebKit
/// otherwise only does for a user tap. Process-wide and applied once: it
/// replaces a method on the WKContentView class, not on an instance.
+ (void)disableKeyboardDisplayRequiresUserAction:(WKWebView *)webView;

@end

NS_ASSUME_NONNULL_END
