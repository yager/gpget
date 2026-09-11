//go:build darwin

#import <AppKit/AppKit.h>
#include <stdio.h>
#include <math.h>

// Go exports (see indicator_darwin.go).
extern void gpgetIndicatorOnSyncNow(void);
extern void gpgetIndicatorOnOpenDest(void);
extern void gpgetIndicatorOnOpenLog(void);
extern void gpgetIndicatorOnQuit(void);

@interface GPGetIndicatorTarget : NSObject
- (void)syncNow:(id)sender;
- (void)openDest:(id)sender;
- (void)openLog:(id)sender;
- (void)showAbout:(id)sender;
- (void)quitGpget:(id)sender;
@end

static NSStatusItem *gItem = nil;
static GPGetIndicatorTarget *gTarget = nil;
static NSMenu *gMenu = nil;
static NSMenuItem *gStatusItem = nil;
static NSMenuItem *gSyncItem = nil;
static BOOL gBusy = NO;
static NSImage *gIdleIcon = nil;

@implementation GPGetIndicatorTarget
- (void)syncNow:(id)sender  { (void)sender; gpgetIndicatorOnSyncNow(); }
- (void)openDest:(id)sender { (void)sender; gpgetIndicatorOnOpenDest(); }
- (void)openLog:(id)sender  { (void)sender; gpgetIndicatorOnOpenLog(); }
- (void)showAbout:(id)sender {
    (void)sender;
    [NSApp activateIgnoringOtherApps:YES];
    [NSApp orderFrontStandardAboutPanel:nil];
}
- (void)quitGpget:(id)sender {
    (void)sender;
    [NSApp activateIgnoringOtherApps:YES];
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"Quit gpget?";
    alert.informativeText = @"gpget will stop watching for your GoPro until you log "
                              "in again. Nothing is uninstalled -- it starts again "
                              "automatically at your next login.";
    [alert addButtonWithTitle:@"Quit"];
    [alert addButtonWithTitle:@"Cancel"];
    if ([alert runModal] == NSAlertFirstButtonReturn) {
        gpgetIndicatorOnQuit();
    }
}
@end

static void setBusy(BOOL busy) {
    gBusy = busy;
    if (gSyncItem != nil) {
        gSyncItem.enabled = !busy;
    }
}

static void onMain(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
        return;
    }
    dispatch_async(dispatch_get_main_queue(), block);
}

// Standard NSStatusBarButton.toolTip — no custom subview or window. A
// covering subview was tried previously to work around a (never confirmed)
// belief that native tooltips don't show for an LSUIElement agent; it also
// silently broke Cmd-drag reordering, which the OS handles for free as long
// as nothing intercepts the button's own mouseDown. Do not reintroduce it.
static void applyTip(NSString *detail) {
    NSString *tip = (detail != nil && detail.length > 0)
        ? [NSString stringWithFormat:@"gpget — %@", detail]
        : @"gpget";
    if (gItem.button != nil) {
        gItem.button.toolTip = tip;
        gItem.button.accessibilityTitle = tip;
        gItem.button.accessibilityLabel = tip;
    }
    if (gStatusItem != nil) {
        gStatusItem.title = (detail != nil && detail.length > 0) ? detail : @"Waiting for a GoPro";
    }
}

static void ensureApp(void) {
    NSApplication *app = [NSApplication sharedApplication];
    [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
    [app finishLaunching];
}

// Action-cam front (inspired by structure only — do not copy GoPro artwork):
// horizontal body, small square front-display outline top-left, larger rounded
// square with hollow circular lens top-right, both top-aligned. No top buttons.
// Chevron under the body for "offload".
static NSImage *idleIcon(void) {
    if (gIdleIcon != nil) return gIdleIcon;
    // Menu-bar slot is ~18–19pt tall; go a bit wider for the landscape body.
    const CGFloat W = 22.0, H = 18.0;
    gIdleIcon = [NSImage imageWithSize:NSMakeSize(W, H) flipped:YES drawingHandler:^BOOL(NSRect dst) {
        (void)dst;
        CGContextRef ctx = [[NSGraphicsContext currentContext] CGContext];
        [[NSColor blackColor] set];

        // Horizontal body (≈3:2), flat top — no rail/bump.
        NSRect body = NSMakeRect(1.0, 1.5, 20.0, 12.0);
        [[NSBezierPath bezierPathWithRoundedRect:body xRadius:2.0 yRadius:2.0] fill];

        // Front display (LEFT): 7×7, margin 2 from body top+left.
        // Punch only the rim (縁=0.7); keep the interior filled (中身は残す).
        const CGFloat dispMargin = 2.0;
        const CGFloat dispS = 7.0;
        NSRect disp = NSMakeRect(NSMinX(body) + dispMargin,
                                 NSMinY(body) + dispMargin,
                                 dispS, dispS);
        const CGFloat rim = 0.7;
        NSBezierPath *dispRing = [NSBezierPath bezierPath];
        [dispRing appendBezierPathWithRoundedRect:disp xRadius:0.8 yRadius:0.8];
        [dispRing appendBezierPathWithRoundedRect:NSInsetRect(disp, rim, rim)
                                          xRadius:0.4
                                          yRadius:0.4];
        dispRing.windingRule = NSEvenOddWindingRule;
        CGContextSetBlendMode(ctx, kCGBlendModeDestinationOut);
        [dispRing fill];
        CGContextSetBlendMode(ctx, kCGBlendModeNormal);
        [[NSColor blackColor] set];

        // Lens (RIGHT): plain circular cutout — ILS/MFT is a simple round mount,
        // not the favicon's square-framed look. 1pt from top + right edges.
        const CGFloat edge = 1.0;
        const CGFloat lensD = 9.0;
        NSRect lens = NSMakeRect(NSMaxX(body) - edge - lensD,
                                 NSMinY(body) + edge,
                                 lensD, lensD);
        CGContextSetBlendMode(ctx, kCGBlendModeDestinationOut);
        [[NSBezierPath bezierPathWithOvalInRect:lens] fill];
        CGContextSetBlendMode(ctx, kCGBlendModeNormal);
        [[NSColor blackColor] set];

        // Down chevron under the body — exactly 1pt gap below the body.
        const CGFloat gap = 1.0;
        const CGFloat chevTop = NSMaxY(body) + gap;
        const CGFloat chevH = 3.4;
        NSBezierPath *v = [NSBezierPath bezierPath];
        [v moveToPoint:NSMakePoint(W / 2.0, chevTop + chevH)];
        [v lineToPoint:NSMakePoint(W / 2.0 - 3.4, chevTop)];
        [v lineToPoint:NSMakePoint(W / 2.0 + 3.4, chevTop)];
        [v closePath];
        [v fill];

        return YES;
    }];
    gIdleIcon.template = YES;
    return gIdleIcon;
}

static NSImage *progressImage(int done, int total) {
    NSString *top = total > 0 ? [NSString stringWithFormat:@"%d", done] : @"…";
    NSString *bot = total > 0 ? [NSString stringWithFormat:@"%d", total] : @"…";

    NSFont *font = [NSFont monospacedDigitSystemFontOfSize:9.0 weight:NSFontWeightMedium];
    NSDictionary *attrs = @{
        NSFontAttributeName: font,
        NSForegroundColorAttributeName: [NSColor blackColor],
    };
    NSSize topSz = [top sizeWithAttributes:attrs];
    NSSize botSz = [bot sizeWithAttributes:attrs];
    // Widen for up to 4-digit counts (e.g. "9999" / "9999") -- the item
    // auto-sizes (NSVariableStatusItemLength in showProgress), so nothing
    // clips as a transfer grows past three digits.
    CGFloat w = ceil(fmax(topSz.width, botSz.width) + 2.0);
    if (w < 12.0) w = 12.0;

    // -drawAtPoint: anchors a string at the *top* of its layout box in a
    // flipped context, and that box carries ~2.5pt of ascent padding above
    // the ink before any pixel is actually drawn (digits have no
    // descenders, so nothing plays the same role at a box's bottom edge).
    // Above the divider that padding lands harmlessly against the canvas
    // edge; below the divider it lands right under the line, so botY has to
    // be pulled up to compensate or the bottom gap reads much bigger than
    // the top one. Measured by counting ink rows in a rendered PNG (see
    // gpgetIndicatorWritePreview), not eyeballed: this keeps ~1.75pt clear
    // on both sides of the divider (up from an earlier ~0.8pt that read as
    // too tight) without touching the font size, at the cost of a slightly
    // taller icon (21pt vs the idle icon's 18pt, which itself renders with
    // headroom to spare in the real menu bar).
    CGFloat topY = 0.0;
    CGFloat midY = 10.8;
    CGFloat botY = 10.1;
    CGFloat bottomMargin = 1.8; // clearance after the "total" row's ink
    CGFloat h = botY + 9.1 /* padding+ink for one row */ + bottomMargin;

    NSImage *img = [NSImage imageWithSize:NSMakeSize(w, h) flipped:YES drawingHandler:^BOOL(NSRect dst) {
        (void)dst;
        [top drawAtPoint:NSMakePoint(floor((w - topSz.width) / 2.0), topY) withAttributes:attrs];
        [bot drawAtPoint:NSMakePoint(floor((w - botSz.width) / 2.0), botY) withAttributes:attrs];

        // Thin divider between the done (top) and total (bottom) rows.
        NSBezierPath *divider = [NSBezierPath bezierPath];
        [divider moveToPoint:NSMakePoint(1.0, midY)];
        [divider lineToPoint:NSMakePoint(w - 1.0, midY)];
        divider.lineWidth = 0.5;
        [[NSColor blackColor] setStroke];
        [divider stroke];

        return YES;
    }];
    img.template = YES;
    return img;
}

static void showIdle(NSString *detail) {
    if (gItem == nil) return;
    NSStatusBarButton *b = gItem.button;
    b.title = @"";
    b.image = idleIcon();
    b.imagePosition = NSImageOnly;
    gItem.length = NSSquareStatusItemLength;
    applyTip(detail);
    setBusy(NO);
}

static void showProgress(int done, int total, NSString *detail) {
    if (gItem == nil) return;
    NSStatusBarButton *b = gItem.button;
    b.title = @"";
    b.image = progressImage(done, total);
    b.imagePosition = NSImageOnly;
    gItem.length = NSVariableStatusItemLength;
    NSString *tipDetail = detail;
    if (tipDetail.length == 0 && total > 0) {
        tipDetail = [NSString stringWithFormat:@"%d/%d files", done, total];
    }
    applyTip(tipDetail);
    setBusy(YES);
}

static void createStatusItem(void) {
    if (gItem != nil) {
        return;
    }
    ensureApp();
    gTarget = [[GPGetIndicatorTarget alloc] init];

    gItem = [[NSStatusBar systemStatusBar]
        statusItemWithLength:NSSquareStatusItemLength];
    gItem.visible = YES;

    gMenu = [[NSMenu alloc] initWithTitle:@"gpget"];
    gStatusItem = [[NSMenuItem alloc] initWithTitle:@"Waiting for a GoPro"
                                             action:nil
                                      keyEquivalent:@""];
    gStatusItem.enabled = NO;
    [gMenu addItem:gStatusItem];
    [gMenu addItem:[NSMenuItem separatorItem]];

    gSyncItem = [[NSMenuItem alloc] initWithTitle:@"Sync Now"
                                           action:@selector(syncNow:)
                                    keyEquivalent:@""];
    gSyncItem.target = gTarget;
    [gMenu addItem:gSyncItem];

    NSMenuItem *dest = [[NSMenuItem alloc] initWithTitle:@"Open Destination"
                                                  action:@selector(openDest:)
                                           keyEquivalent:@""];
    dest.target = gTarget;
    [gMenu addItem:dest];

    NSMenuItem *log = [[NSMenuItem alloc] initWithTitle:@"Show Log"
                                                 action:@selector(openLog:)
                                          keyEquivalent:@""];
    log.target = gTarget;
    [gMenu addItem:log];

    [gMenu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *about = [[NSMenuItem alloc] initWithTitle:@"About gpget"
                                                    action:@selector(showAbout:)
                                             keyEquivalent:@""];
    about.target = gTarget;
    [gMenu addItem:about];

    [gMenu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit gpget"
                                                   action:@selector(quitGpget:)
                                            keyEquivalent:@""];
    quit.target = gTarget;
    [gMenu addItem:quit];

    // Assigning .menu makes AppKit pop it up on click by itself, and leaves
    // the button's own mouseDown handling (incl. Cmd-drag reordering) intact.
    gItem.menu = gMenu;

    showIdle(@"waiting for a GoPro");
    fprintf(stderr, "[indicator] menu bar item ready (native tooltip + menu)\n");
}

void gpgetIndicatorStart(void) {
    if ([NSThread isMainThread]) {
        createStatusItem();
        return;
    }
    fprintf(stderr, "[indicator] Start called off the main thread — "
                    "status item may never appear\n");
    dispatch_async(dispatch_get_main_queue(), ^{
        createStatusItem();
    });
}

void gpgetIndicatorSetIdle(const char *last) {
    NSString *note = last && last[0] ? [NSString stringWithUTF8String:last] : @"waiting for a GoPro";
    onMain(^{ showIdle(note); });
}

void gpgetIndicatorSetProgress(int done, int total, const char *detail) {
    NSString *extra = detail && detail[0] ? [NSString stringWithUTF8String:detail] : @"";
    onMain(^{ showProgress(done, total, extra); });
}

void gpgetIndicatorSetMessage(const char *msg) {
    NSString *s = msg && msg[0] ? [NSString stringWithUTF8String:msg] : @"gpget";
    onMain(^{ showIdle(s); });
}

// writePNG draws img onto a light canvas (so black template art is visible)
// at `scale`, and writes path as PNG. Returns YES on success.
static BOOL writePNG(NSImage *img, NSString *path, CGFloat scale) {
    if (img == nil || path.length == 0 || scale < 1.0) return NO;
    NSSize src = img.size;
    NSSize out = NSMakeSize(ceil(src.width * scale), ceil(src.height * scale));
    NSImage *canvas = [[NSImage alloc] initWithSize:out];
    [canvas lockFocus];
    [[NSColor colorWithCalibratedWhite:0.92 alpha:1.0] set];
    NSRectFill(NSMakeRect(0, 0, out.width, out.height));
    // Hairline grid so 1x size is obvious when viewing large previews.
    if (scale >= 4.0) {
        [[NSColor colorWithCalibratedWhite:0.85 alpha:1.0] set];
        NSBezierPath *grid = [NSBezierPath bezierPath];
        grid.lineWidth = 1.0;
        for (CGFloat x = 0; x <= out.width; x += scale) {
            [grid moveToPoint:NSMakePoint(x + 0.5, 0)];
            [grid lineToPoint:NSMakePoint(x + 0.5, out.height)];
        }
        for (CGFloat y = 0; y <= out.height; y += scale) {
            [grid moveToPoint:NSMakePoint(0, y + 0.5)];
            [grid lineToPoint:NSMakePoint(out.width, y + 0.5)];
        }
        [grid stroke];
    }
    [img drawInRect:NSMakeRect(0, 0, out.width, out.height)
           fromRect:NSZeroRect
          operation:NSCompositingOperationSourceOver
           fraction:1.0
     respectFlipped:YES
              hints:@{NSImageHintInterpolation: @(NSImageInterpolationNone)}];
    [canvas unlockFocus];

    NSData *tiff = [canvas TIFFRepresentation];
    if (tiff == nil) return NO;
    NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
    if (rep == nil) return NO;
    NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
    if (png == nil) return NO;
    return [png writeToFile:path atomically:YES];
}

// gpgetIndicatorWritePreview writes review PNGs without touching the menu bar.
// Files (under dir):
//   idle_1x.png / idle_8x.png
//   progress_12_847_1x.png / progress_12_847_8x.png
//   progress_1234_9999_1x.png / progress_1234_9999_8x.png (4-digit stress case)
int gpgetIndicatorWritePreview(const char *dir) {
    @autoreleasepool {
        if (dir == NULL || dir[0] == '\0') return 0;
        NSString *d = [NSString stringWithUTF8String:dir];
        NSFileManager *fm = [NSFileManager defaultManager];
        NSError *err = nil;
        if (![fm createDirectoryAtPath:d withIntermediateDirectories:YES attributes:nil error:&err]) {
            fprintf(stderr, "[indicator] preview mkdir: %s\n",
                    err ? [[err localizedDescription] UTF8String] : "(unknown)");
            return 0;
        }
        // Bust the idle cache so edits to idleIcon() show up immediately.
        gIdleIcon = nil;
        NSImage *idle = idleIcon();
        NSImage *prog = progressImage(12, 847);
        NSImage *prog4 = progressImage(1234, 9999);
        BOOL ok = YES;
        ok = writePNG(idle, [d stringByAppendingPathComponent:@"idle_1x.png"], 1.0) && ok;
        ok = writePNG(idle, [d stringByAppendingPathComponent:@"idle_8x.png"], 8.0) && ok;
        ok = writePNG(prog, [d stringByAppendingPathComponent:@"progress_12_847_1x.png"], 1.0) && ok;
        ok = writePNG(prog, [d stringByAppendingPathComponent:@"progress_12_847_8x.png"], 8.0) && ok;
        ok = writePNG(prog4, [d stringByAppendingPathComponent:@"progress_1234_9999_1x.png"], 1.0) && ok;
        ok = writePNG(prog4, [d stringByAppendingPathComponent:@"progress_1234_9999_8x.png"], 8.0) && ok;
        if (!ok) {
            fprintf(stderr, "[indicator] preview write failed under %s\n", dir);
            return 0;
        }
        fprintf(stderr, "[indicator] preview written under %s\n", dir);
        return 1;
    }
}
