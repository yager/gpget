//go:build darwin

#import <AppKit/AppKit.h>
#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdio.h>

// Implemented in Go (see watch_darwin.go).
extern void gpgetUSBAttached(void);
extern void gpgetUSBDetached(void);

// An IOKit iterator must be drained for the notification to re-arm, so these
// always run the loop even when they ignore the contents.
static void attachedCB(void *refcon, io_iterator_t iter) {
    (void)refcon;
    io_service_t s; int n = 0;
    while ((s = IOIteratorNext(iter))) { IOObjectRelease(s); n++; }
    if (n > 0) gpgetUSBAttached();
}

static void detachedCB(void *refcon, io_iterator_t iter) {
    (void)refcon;
    io_service_t s; int n = 0;
    while ((s = IOIteratorNext(iter))) { IOObjectRelease(s); n++; }
    if (n > 0) gpgetUSBDetached();
}

// deviceMatch builds {IOProviderClass: IOUSBHostDevice, IOPropertyMatch: {idVendor: N}}.
//
// The vendor filter MUST sit inside IOPropertyMatch. A top-level "idVendor" key
// matches nothing on macOS 15 -- measured 2026-09-05: class alone returns 18
// devices, class + top-level idVendor returns 0, class + IOPropertyMatch
// returns exactly the camera.
static CFMutableDictionaryRef deviceMatch(int vendorID) {
    CFMutableDictionaryRef d = IOServiceMatching("IOUSBHostDevice");
    if (!d) return NULL;
    CFMutableDictionaryRef props = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFNumberRef v = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &vendorID);
    CFDictionarySetValue(props, CFSTR("idVendor"), v);
    CFRelease(v);
    CFDictionarySetValue(d, CFSTR("IOPropertyMatch"), props);
    CFRelease(props);
    return d;
}

// gpgetWatchUSB blocks forever, calling into Go when a matching USB device
// arrives or leaves. Returns 0 only if setup failed.
//
// The blocking call matters: if the indicator package has already created an
// NSApplication (menu-bar agent mode), we must pump it with [NSApp run]
// rather than a bare CFRunLoopRun(). AppKit can still create and draw a
// window (NSStatusItem included) without its event loop ever running --
// windows are ordered onto the screen directly -- but mouse clicks, hover
// tracking and Cmd-drag reordering are all delivered through NSApplication's
// own event dispatch, which only starts once something pumps it (normally
// -run). A plain CFRunLoopRun() drains our IOKit source fine but never touches
// that dispatch, so the status item renders yet is completely inert. [NSApp
// run] pumps NSDefaultRunLoopMode, the same mode constant as
// kCFRunLoopDefaultMode used below, so the IOKit source still fires.
int gpgetWatchUSB(int vendorID) {
    IONotificationPortRef port = IONotificationPortCreate(kIOMainPortDefault);
    if (!port) return 0;
    CFRunLoopAddSource(CFRunLoopGetCurrent(),
                       IONotificationPortGetRunLoopSource(port),
                       kCFRunLoopDefaultMode);

    CFMutableDictionaryRef match = deviceMatch(vendorID);
    if (!match) return 0;
    CFRetain(match); // each AddMatchingNotification consumes one reference

    io_iterator_t addedIter = 0, removedIter = 0;
    if (IOServiceAddMatchingNotification(port, kIOMatchedNotification, match,
                                         attachedCB, NULL, &addedIter) != KERN_SUCCESS) {
        return 0;
    }
    // Draining arms the notification. It also reports a camera that was already
    // plugged in when we started, which is what we want at login.
    attachedCB(NULL, addedIter);

    if (IOServiceAddMatchingNotification(port, kIOTerminatedNotification, match,
                                         detachedCB, NULL, &removedIter) != KERN_SUCCESS) {
        return 0;
    }
    detachedCB(NULL, removedIter);

    if (NSApp != nil) {
        [NSApp run];
    } else {
        CFRunLoopRun();
    }
    return 1;
}
