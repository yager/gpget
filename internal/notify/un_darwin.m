//go:build darwin

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#include <stdio.h>

// pump runs the main run loop until *done or the deadline passes.
//
// This must not be a semaphore wait. The authorization prompt is presented on
// the main thread, so blocking it means the prompt can never appear -- macOS
// then records a *denial* that persists, and every later attempt fails with
// "Notifications are not allowed for this application". (Learned the hard way
// on 2026-09-05.)
static BOOL pump(volatile BOOL *done, double seconds) {
    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:seconds];
    while (!*done && [deadline timeIntervalSinceNow] > 0) {
        [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                 beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
    }
    return *done;
}

// gpgetNotifyNative posts a banner attributed to *this* app bundle, the way any
// normal Mac app does. It only works from inside a bundle with an identifier;
// from a bare executable UNUserNotificationCenter raises, hence the @try.
//
// Returns 1 if the notification was accepted, 0 otherwise. Diagnostics go to
// stderr, which in autostart context is the log file.
//
// cid, when non-empty, is the UNNotificationRequest identifier so a live
// transfer can replace its own banner instead of stacking a new one each tick.
int gpgetNotifyNative(const char *cid, const char *ctitle, const char *cbody) {
    @autoreleasepool {
        @try {
            // currentNotificationCenter raises NSInternalInconsistencyException
            // when the process has no bundle identity, and the raise happens
            // inside a dispatch_once block. libdispatch does not let exceptions
            // unwind through its callouts, so it calls terminate() and the
            // @catch below never runs -- the process dies. Observed on a fresh
            // account running the bare binary: "bundleProxyForCurrentProcess is
            // nil". The only safe move is not to call it at all.
            NSString *bundleID = [[NSBundle mainBundle] bundleIdentifier];
            if (bundleID == nil) {
                fprintf(stderr, "[un] not running from an app bundle; "
                                "UserNotifications is unavailable here\n");
                return 0;
            }

            UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
            if (c == nil) { fprintf(stderr, "[un] no notification center\n"); return 0; }

            __block volatile BOOL d0 = NO;
            __block int authStatus = -1;
            [c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *st) {
                authStatus = (int)st.authorizationStatus;
                d0 = YES;
            }];
            pump(&d0, 10.0);

            // Only narrate when something is wrong; a working run should not
            // pollute the log on every camera connection.
            if (authStatus == 1) {
                fprintf(stderr, "[un] notifications are denied (bundleID=%s) — "
                                "turn gpget on in System Settings > Notifications\n",
                        [[[NSBundle mainBundle] bundleIdentifier] UTF8String] ?: "(nil)");
                return 0;
            }

            __block volatile BOOL d1 = NO;
            __block BOOL granted = NO;
            __block NSString *authErr = nil;
            [c requestAuthorizationWithOptions:UNAuthorizationOptionAlert
                             completionHandler:^(BOOL g, NSError *e) {
                granted = g;
                if (e) authErr = [e localizedDescription];
                d1 = YES;
            }];
            if (!pump(&d1, 60.0)) { fprintf(stderr, "[un] requestAuthorization: timeout\n"); return 0; }
            if (!granted) {
                fprintf(stderr, "[un] requestAuthorization: granted=NO err=%s\n",
                        authErr ? [authErr UTF8String] : "(none)");
                return 0;
            }

            UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
            content.title = [NSString stringWithUTF8String:ctitle];
            content.body  = [NSString stringWithUTF8String:cbody];

            NSString *ident;
            if (cid && cid[0] != '\0') {
                ident = [NSString stringWithUTF8String:cid];
            } else {
                ident = [[NSUUID UUID] UUIDString];
            }
            UNNotificationRequest *req =
                [UNNotificationRequest requestWithIdentifier:ident
                                                     content:content
                                                     trigger:nil];
            __block volatile BOOL d2 = NO;
            __block BOOL ok = NO;
            __block NSString *addErr = nil;
            [c addNotificationRequest:req withCompletionHandler:^(NSError *e) {
                ok = (e == nil);
                if (e) addErr = [e localizedDescription];
                d2 = YES;
            }];
            if (!pump(&d2, 20.0)) { fprintf(stderr, "[un] addNotificationRequest: timeout\n"); return 0; }
            if (!ok) fprintf(stderr, "[un] addNotificationRequest: %s\n",
                             addErr ? [addErr UTF8String] : "(unknown)");
            return ok ? 1 : 0;
        } @catch (NSException *ex) {
            fprintf(stderr, "[un] exception: %s\n", [[ex reason] UTF8String] ?: "(nil)");
            return 0;
        }
    }
}
