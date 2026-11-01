#import <AppKit/AppKit.h>

static void record(NSRunningApplication *application) {
    NSDictionary *event = @{
        @"at_ms": @((long long)(NSDate.date.timeIntervalSince1970 * 1000)),
        @"pid": @(application.processIdentifier),
        @"bundle": application.bundleIdentifier ?: @"unknown"
    };
    NSData *data = [NSJSONSerialization dataWithJSONObject:event options:0 error:nil];
    fwrite(data.bytes, 1, data.length, stdout);
    fputc('\n', stdout);
    fflush(stdout);
}

int main(void) {
    @autoreleasepool {
        NSWorkspace *workspace = NSWorkspace.sharedWorkspace;
        record(workspace.frontmostApplication);
        [workspace.notificationCenter addObserverForName:NSWorkspaceDidActivateApplicationNotification
            object:nil queue:NSOperationQueue.mainQueue usingBlock:^(NSNotification *note) {
                record(note.userInfo[NSWorkspaceApplicationKey]);
            }];
        [NSRunLoop.currentRunLoop run];
    }
    return 0;
}
