#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

/**
 * Thin Objective-C shim around React's C reload primitive
 * (`RCTTriggerReloadCommandListeners`, from <React/RCTReloadCommand.h>).
 *
 * The C function is declared with RCT_EXTERN and is therefore not directly
 * importable into Swift; wrapping it in a class method makes it visible to the
 * Swift module via this pod's generated umbrella header / module map.
 */
@interface AppUpdaterReload : NSObject

/** Triggers a React Native reload for all registered listeners. */
+ (void)triggerWithReason:(NSString *)reason;

@end

NS_ASSUME_NONNULL_END
