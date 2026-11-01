#import <AppKit/AppKit.h>
#import "../../internal/ui/sessionkeys_darwin.h"

@interface KazeKeyProbe : NSView
@property NSUInteger downs;
@property NSUInteger ups;
@property NSEventModifierFlags lastMods;
@end
@implementation KazeKeyProbe
- (BOOL)acceptsFirstResponder { return YES; }
- (void)keyDown:(NSEvent *)e { self.downs++; self.lastMods = e.modifierFlags; }
- (void)keyUp:(NSEvent *)e { self.ups++; }
@end

static NSEvent *keyEvent(NSWindow *w, NSEventType type, NSEventModifierFlags mods, unsigned short code) {
	return [NSEvent keyEventWithType:type location:NSZeroPoint modifierFlags:mods
		timestamp:0 windowNumber:w.windowNumber context:nil characters:@"\t"
		charactersIgnoringModifiers:@"\t" isARepeat:NO keyCode:code];
}

static void check(BOOL condition, const char *message) {
	if (!condition) { fprintf(stderr, "FAIL: %s\n", message); exit(1); }
}

int main(void) { @autoreleasepool {
	[NSApplication sharedApplication];
	// These windows stay hidden. The test never activates or posts global input.
	[NSApp setActivationPolicy:NSApplicationActivationPolicyProhibited];
	NSWindow *w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 400, 300)
		styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable backing:NSBackingStoreBuffered defer:NO];
	KazeKeyProbe *v = [[KazeKeyProbe alloc] initWithFrame:NSMakeRect(0, 0, 400, 300)];
	w.contentView = v;
	[w makeFirstResponder:v];
	// Reproduce AppKit's interception without the bridge.
	[w sendEvent:keyEvent(w, NSEventTypeKeyDown, NSEventModifierFlagControl, 48)];
	check(v.downs == 0, "baseline must reproduce AppKit consuming Control+Tab");
	id monitor = kzMonitorSessionKeys(v);
	check(monitor != nil, "local session-key monitor was not installed");
	for (NSNumber *flags in @[@(NSEventModifierFlagControl), @(NSEventModifierFlagControl | NSEventModifierFlagShift)]) {
		NSUInteger before = v.downs;
		[NSApp sendEvent:keyEvent(w, NSEventTypeKeyDown, flags.unsignedLongValue, 48)];
		check(v.downs == before + 1, "session shortcut must reach the view exactly once");
		check(v.lastMods == flags.unsignedLongValue, "shortcut modifiers changed");
		[NSApp sendEvent:keyEvent(w, NSEventTypeKeyUp, flags.unsignedLongValue, 48)];
	}
	check(v.ups == 2, "shortcut releases did not reach the view");
	NSWindow *other = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 100, 100)
		styleMask:NSWindowStyleMaskTitled backing:NSBackingStoreBuffered defer:NO];
	[NSApp sendEvent:keyEvent(other, NSEventTypeKeyDown, NSEventModifierFlagControl, 48)];
	check(v.downs == 2, "another window's event reached Kaze");
	// These are not session shortcuts. Hidden windows otherwise receive no keys
	// from NSApplication, so any extra delivery would be the monitor's doing.
	for (NSNumber *flags in @[@0, @(NSEventModifierFlagShift), @(NSEventModifierFlagControl | NSEventModifierFlagOption), @(NSEventModifierFlagControl | NSEventModifierFlagCommand)]) {
		[NSApp sendEvent:keyEvent(w, NSEventTypeKeyDown, flags.unsignedLongValue, 48)];
	}
	[NSApp sendEvent:keyEvent(w, NSEventTypeKeyDown, NSEventModifierFlagControl, 0)];
	check(v.downs == 2, "ordinary keys were intercepted");
	[NSEvent removeMonitor:monitor];
	[NSApp sendEvent:keyEvent(w, NSEventTypeKeyDown, NSEventModifierFlagControl, 48)];
	check(v.downs == 2, "removed monitor still delivered a shortcut");
	check(!w.visible && !other.visible && !w.keyWindow, "test windows must remain hidden and unfocused");
	puts("PASS: macOS session-key forwarding, isolation and cleanup");
	return 0;
}}
