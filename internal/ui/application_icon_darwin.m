//go:build darwin && cgo

#import <AppKit/AppKit.h>

void superlink_application_icon(const void *bytes, size_t length) {
    NSData *data = [[NSData alloc] initWithBytes:bytes length:length];
    NSImage *image = [[NSImage alloc] initWithData:data];
    if (image) {
        void (^apply)(void) = ^{ [NSApplication sharedApplication].applicationIconImage = image; };
        if ([NSThread isMainThread]) apply();
        else dispatch_sync(dispatch_get_main_queue(), apply);
    }
    [image release];
    [data release];
}
