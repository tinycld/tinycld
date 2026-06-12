#import "AppUpdaterReload.h"

#import <React/RCTReloadCommand.h>

@implementation AppUpdaterReload

+ (void)triggerWithReason:(NSString *)reason
{
    RCTTriggerReloadCommandListeners(reason);
}

@end
