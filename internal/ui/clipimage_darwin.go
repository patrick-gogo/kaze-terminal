//go:build darwin && !ios

package ui

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// kzClipHasImage reports whether the general pasteboard holds image data, such
// as a screenshot copied with Cmd+Ctrl+Shift+4. It reads only the type list.
static int kzClipHasImage(void) {
	@autoreleasepool {
		NSPasteboard *p = [NSPasteboard generalPasteboard];
		return [p availableTypeFromArray:@[NSPasteboardTypePNG, NSPasteboardTypeTIFF]] != nil;
	}
}
*/
import "C"

// systemClipImage reports whether the system clipboard holds an image.
func systemClipImage() bool { return C.kzClipHasImage() != 0 }
