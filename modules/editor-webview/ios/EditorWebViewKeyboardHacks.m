#import "EditorWebViewKeyboardHacks.h"
#import <objc/runtime.h>
#import <UIKit/UIKit.h>

// Port of react-native-webview's `_SwizzleHelperWK` (apple/RNCWebViewImpl.m).
// Its one method is copied onto a runtime subclass of WKContentView, so `self`
// here is that content view. Returning nil removes the accessory bar; clearing
// the assistant item's groups removes the iPad shortcuts bar.
@interface _TinyCldSwizzleHelperWK : UIView
@end

@implementation _TinyCldSwizzleHelperWK

- (id)inputAccessoryView
{
  if ([self respondsToSelector:@selector(inputAssistantItem)]) {
    UITextInputAssistantItem *item = [self inputAssistantItem];
    item.leadingBarButtonGroups = @[];
    item.trailingBarButtonGroups = @[];
  }
  return nil;
}

@end

static NSString *const kSwizzleSuffix = @"_TinyCldSwizzleHelperWK";

// WebKit keeps its content view (class WKContentView) as a subview of the web
// view's scroll view. Both hacks target it.
static UIView *EditorWebViewContentView(WKWebView *webView)
{
  UIView *found = nil;
  for (UIView *view in webView.scrollView.subviews) {
    if ([NSStringFromClass(view.class) hasPrefix:@"WK"]) {
      found = view;
    }
  }
  return found;
}

@implementation EditorWebViewKeyboardHacks

+ (void)hideInputAccessoryView:(WKWebView *)webView
{
  UIView *subview = EditorWebViewContentView(webView);
  if (subview == nil) {
    return;
  }
  if ([NSStringFromClass(subview.class) hasSuffix:kSwizzleSuffix]) {
    return;
  }

  // A distinct suffix from react-native-webview's `_SwizzleHelperWK`: both
  // libraries live in this binary, and sharing a registered class name would
  // hand one of them the other's subclass.
  NSString *name = [NSStringFromClass(subview.class) stringByAppendingString:kSwizzleSuffix];
  Class newClass = NSClassFromString(name);

  if (newClass == nil) {
    newClass = objc_allocateClassPair(subview.class, name.UTF8String, 0);
    if (!newClass) {
      return;
    }
    Method method = class_getInstanceMethod([_TinyCldSwizzleHelperWK class], @selector(inputAccessoryView));
    class_addMethod(newClass, @selector(inputAccessoryView), method_getImplementation(method), method_getTypeEncoding(method));
    objc_registerClassPair(newClass);
  }

  object_setClass(subview, newClass);
}

+ (void)disableKeyboardDisplayRequiresUserAction:(WKWebView *)webView
{
  UIView *subview = EditorWebViewContentView(webView);
  if (subview == nil) {
    return;
  }
  Class contentViewClass = subview.class;

  // Once, not per instance as react-native-webview does: `method_setImplementation`
  // rewrites the class's method, so a second application would wrap the first
  // override rather than the original.
  static dispatch_once_t onceToken;
  dispatch_once(&onceToken, ^{
    // The iOS 13+ selector. Deployment target is 15.1, so the older spellings
    // react-native-webview still carries are unreachable here.
    SEL selector = sel_getUid("_elementDidFocus:userIsInteracting:blurPreviousNode:activityStateChanges:userObject:");
    Method method = class_getInstanceMethod(contentViewClass, selector);
    if (method == NULL) {
      // WebKit renamed it. Programmatic focus then needs a tap, which is the
      // stock behaviour — degrade, don't crash.
      return;
    }
    IMP original = method_getImplementation(method);
    IMP override = imp_implementationWithBlock(^void(id me, void *arg0, __unused BOOL arg1, BOOL arg2, BOOL arg3, id arg4) {
      ((void (*)(id, SEL, void *, BOOL, BOOL, BOOL, id))original)(me, selector, arg0, YES, arg2, arg3, arg4);
    });
    method_setImplementation(method, override);
  });
}

@end
