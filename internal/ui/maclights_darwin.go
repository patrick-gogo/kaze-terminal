//go:build darwin && !ios

package ui

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#import "sessionkeys_darwin.h"
#include <stdint.h>

extern void kazeLightsMoved(uint64_t gen, double inset);
extern int kazeTitleDrag(uint64_t gen, double x, double y);

static NSMutableDictionary<NSNumber*, NSArray*> *kzObservers;

// kzPlace shows the standard buttons, makes the titlebar container as tall as
// Kaze's bar and centres the buttons in it, left starting at left points.
// Returns where the buttons end, or 0 in full screen.
static double kzPlace(NSWindow *w, double barH, double left) {
	NSButton *b[3] = {
		[w standardWindowButton:NSWindowCloseButton],
		[w standardWindowButton:NSWindowMiniaturizeButton],
		[w standardWindowButton:NSWindowZoomButton],
	};
	if (b[0] == nil || b[1] == nil || b[2] == nil) {
		return 0;
	}
	for (int i = 0; i < 3; i++) {
		[b[i] setHidden:NO];
	}
	if (([w styleMask] & NSWindowStyleMaskFullScreen) != 0) {
		return 0; // the system shows its own buttons in full screen
	}
	NSView *container = [[b[0] superview] superview];
	if (container == nil) {
		return 0;
	}
	NSRect r = [container frame];
	r.size.height = barH;
	r.origin.y = NSHeight([w frame]) - barH;
	[container setFrame:r];
	CGFloat gap = NSMinX([b[1] frame]) - NSMinX([b[0] frame]);
	if (gap <= 0) {
		gap = 20;
	}
	CGFloat bh = NSHeight([b[0] frame]);
	CGFloat x = left;
	for (int i = 0; i < 3; i++) {
		[b[i] setFrameOrigin:NSMakePoint(x, (barH - bh) / 2)];
		x += gap;
	}
	return NSMaxX([b[2] frame]);
}

static void kzDetach(uint64_t gen) {
	if (![NSThread isMainThread] || kzObservers == nil) {
		return;
	}
	NSArray *obs = kzObservers[@(gen)];
	if (obs == nil) {
		return;
	}
	for (id o in obs[0]) {
		[[NSNotificationCenter defaultCenter] removeObserver:o];
	}
	for (NSUInteger i = 1; i < [obs count]; i++) {
		[NSEvent removeMonitor:obs[i]];
	}
	[kzObservers removeObjectForKey:@(gen)];
}

static void kzAttach(CFTypeRef viewRef, uint64_t gen, double barH, double left) {
	if (![NSThread isMainThread]) {
		return;
	}
	NSView *view = (__bridge NSView *)viewRef;
	NSWindow *w = [view window];
	if (w == nil) {
		return;
	}
	if (kzObservers == nil) {
		kzObservers = [NSMutableDictionary dictionary];
	}
	if (kzObservers[@(gen)] != nil) {
		return; // already attached for this generation
	}
	__weak NSWindow *weakW = w;
	void (^again)(NSNotification *) = ^(NSNotification *n) {
		NSWindow *ww = weakW;
		if (ww != nil) {
			kazeLightsMoved(gen, kzPlace(ww, barH, left));
		}
	};
	NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
	NSMutableArray *obs = [NSMutableArray array];
	for (NSNotificationName name in @[NSWindowDidResizeNotification, NSWindowDidEnterFullScreenNotification,
	                                  NSWindowDidExitFullScreenNotification, NSWindowDidBecomeKeyNotification]) {
		[obs addObject:[nc addObserverForName:name object:w queue:nil usingBlock:again]];
	}
	[obs addObject:[nc addObserverForName:NSWindowWillCloseNotification object:w queue:nil usingBlock:^(NSNotification *n) {
		kzDetach(gen); // cleans up even when Gio can no longer run work for us
	}]];
	// A double click on empty bar space zooms or minimizes, as the user's
	// macOS setting says. Gio turns presses there into window drags, so Kaze
	// never sees the click; this asks Kaze whether the point is empty bar.
	id mon = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown handler:^NSEvent *(NSEvent *e) {
		NSWindow *ww = weakW;
		if (ww == nil || [e window] != ww || [e clickCount] != 2) {
			return e;
		}
		NSPoint p = [e locationInWindow];
		double fromTop = NSHeight([[ww contentView] frame]) - p.y;
		if (!kazeTitleDrag(gen, p.x, fromTop)) {
			return e;
		}
		NSString *act = [[NSUserDefaults standardUserDefaults] stringForKey:@"AppleActionOnDoubleClick"];
		if ([act isEqualToString:@"Minimize"]) {
			[ww miniaturize:nil];
		} else if (![act isEqualToString:@"None"]) {
			[ww zoom:nil];
		}
		return nil;
	}];
	NSMutableArray *attachment = [NSMutableArray arrayWithObject:obs];
	if (mon != nil) { [attachment addObject:mon]; }
	id keys = kzMonitorSessionKeys(view);
	if (keys != nil) { [attachment addObject:keys]; }
	kzObservers[@(gen)] = attachment;
	kazeLightsMoved(gen, kzPlace(w, barH, left));
}

static void kzReplace(CFTypeRef viewRef, uint64_t gen, double barH, double left) {
	if (![NSThread isMainThread]) {
		return;
	}
	NSWindow *w = [(__bridge NSView *)viewRef window];
	if (w != nil && kzObservers[@(gen)] != nil) {
		kazeLightsMoved(gen, kzPlace(w, barH, left));
	}
}
*/
import "C"

import (
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/io/event"
)

// macView picks the NSView out of Gio's AppKit view event: ok is false for
// any other event, view is 0 for an invalid one.
func macView(e event.Event) (view uintptr, ok bool) {
	v, ok := e.(app.AppKitViewEvent)
	if !ok || !v.Valid() {
		return 0, ok
	}
	return v.View, true
}

// activeBridge is the bridge native callbacks report to; Kaze has one window.
var activeBridge atomic.Pointer[macBridge]

type appKitLights struct{}

func (appKitLights) attach(view uintptr, gen uint64, barH, left float64) {
	C.kzAttach(C.CFTypeRef(view), C.uint64_t(gen), C.double(barH), C.double(left))
}

func (appKitLights) place(view uintptr, gen uint64, barH, left float64) {
	C.kzReplace(C.CFTypeRef(view), C.uint64_t(gen), C.double(barH), C.double(left))
}

func (appKitLights) detach(view uintptr, gen uint64) { C.kzDetach(C.uint64_t(gen)) }

// newMacBridge makes the bridge for this window and routes native callbacks to it.
func newMacBridge(run func(func()), invalidate func(), barH, left float64) *macBridge {
	b := &macBridge{nat: appKitLights{}, run: run, invalidate: invalidate, barH: barH, left: left}
	activeBridge.Store(b)
	return b
}

//export kazeLightsMoved
func kazeLightsMoved(gen C.uint64_t, inset C.double) {
	if b := activeBridge.Load(); b != nil {
		b.publish(uint64(gen), float64(inset))
	}
}

//export kazeTitleDrag
func kazeTitleDrag(gen C.uint64_t, x, y C.double) C.int {
	if b := activeBridge.Load(); b != nil && b.current(uint64(gen)) && b.dragAt(float64(x), float64(y)) {
		return 1
	}
	return 0
}
