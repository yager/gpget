//go:build darwin

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#include <stdio.h>
#include <string.h>

// gpgetNotifySettings writes macOS's own view of this app's notification
// settings into buf. It exists because the System Settings UI and the API can
// disagree -- and because a screenshot of that UI is someone's reading of it,
// while this is the value the notification daemon actually acts on.
//
// Returns the number of bytes written, or 0 if the settings could not be read.
int gpgetNotifySettings(char *buf, int cap) {
    @autoreleasepool {
        if ([[NSBundle mainBundle] bundleIdentifier] == nil) return 0;
        UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
        if (c == nil) return 0;

        __block volatile BOOL done = NO;
        __block NSString *out = nil;
        [c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
            out = [NSString stringWithFormat:
                @"bundle                     %@\n"
                @"authorizationStatus        %ld   (0 notDetermined 1 denied 2 authorized 3 provisional 4 ephemeral)\n"
                @"alertSetting               %ld   (0 notSupported 1 disabled 2 enabled)\n"
                @"alertStyle                 %ld   (0 none 1 banner 2 alert)\n"
                @"notificationCenterSetting  %ld\n"
                @"lockScreenSetting          %ld\n"
                @"soundSetting               %ld\n"
                @"badgeSetting               %ld\n"
                @"showPreviewsSetting        %ld   (0 always 1 whenAuthenticated 2 never)\n"
                @"criticalAlertSetting       %ld\n"
                @"scheduledDeliverySetting   %ld\n"
                @"timeSensitiveSetting       %ld\n"
                @"providesAppNotifSettings   %d\n",
                [[NSBundle mainBundle] bundleIdentifier],
                (long)s.authorizationStatus,
                (long)s.alertSetting,
                (long)s.alertStyle,
                (long)s.notificationCenterSetting,
                (long)s.lockScreenSetting,
                (long)s.soundSetting,
                (long)s.badgeSetting,
                (long)s.showPreviewsSetting,
                (long)s.criticalAlertSetting,
                (long)s.scheduledDeliverySetting,
                (long)s.timeSensitiveSetting,
                (int)s.providesAppNotificationSettings];
            done = YES;
        }];
        NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:10.0];
        while (!done && [deadline timeIntervalSinceNow] > 0) {
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
        }
        if (out == nil) return 0;
        const char *s = [out UTF8String];
        if (s == NULL) return 0;
        int n = (int)strlen(s);
        if (n >= cap) n = cap - 1;
        memcpy(buf, s, n);
        buf[n] = '\0';
        return n;
    }
}
