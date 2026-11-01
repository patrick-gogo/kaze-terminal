#import <AppKit/AppKit.h>

// AppKit consumes Control+Tab before sending keyDown to the content view.
// The returned local monitor is owned and removed by the view's attachment.
static id kzMonitorSessionKeys(NSView *view) {
	__weak NSView *weakView = view;
	__weak NSWindow *weakWindow = view.window;
	return [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown | NSEventMaskKeyUp
		handler:^NSEvent *(NSEvent *e) {
		NSView *target = weakView;
		NSWindow *window = weakWindow;
		if (target == nil || window == nil || e.window != window || target.window != window || e.keyCode != 48) {
			return e;
		}
		NSEventModifierFlags mods = e.modifierFlags & (NSEventModifierFlagControl |
			NSEventModifierFlagShift | NSEventModifierFlagOption | NSEventModifierFlagCommand);
		if (mods != NSEventModifierFlagControl && mods != (NSEventModifierFlagControl | NSEventModifierFlagShift)) {
			return e;
		}
		// Use GioView's normal conversion and focused Go key routing, including
		// configurable shortcuts. Consume the original to avoid double delivery.
		if (e.type == NSEventTypeKeyDown) { [target keyDown:e]; }
		else { [target keyUp:e]; }
		return nil;
	}];
}
