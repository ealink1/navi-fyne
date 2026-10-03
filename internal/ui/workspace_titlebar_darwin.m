//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import <stdint.h>

extern void naviWorkspaceChanged(uintptr_t callback, int mode);
extern void navi_utilities_install(uintptr_t pointer, uintptr_t callback);
extern void navi_utilities_appearance(uintptr_t pointer, int dark);
extern void navi_utilities_remove(uintptr_t pointer);
static const char NaviWorkspaceKey;

@interface NaviWorkspaceTitlebar : NSTitlebarAccessoryViewController
@property(nonatomic, assign) uintptr_t callback;
@property(nonatomic, retain) NSSegmentedControl *selector;
@end

@implementation NaviWorkspaceTitlebar
- (void)selectWorkspace:(NSSegmentedControl *)sender {
    if (self.callback != 0) {
        naviWorkspaceChanged(self.callback, (int)sender.selectedSegment);
    }
}
- (void)dealloc {
    [_selector release];
    [super dealloc];
}
@end

static void NaviOnMain(void (^run)(void)) {
    if ([NSThread isMainThread]) run();
    else dispatch_sync(dispatch_get_main_queue(), run);
}

int navi_workspace_install(uintptr_t pointer, uintptr_t callback) {
    __block int installed = 0;
    NaviOnMain(^{
        NSWindow *window = (NSWindow *)pointer;
        if (!window || objc_getAssociatedObject(window, &NaviWorkspaceKey)) return;
        NaviWorkspaceTitlebar *accessory = [[NaviWorkspaceTitlebar alloc] init];
        accessory.callback = callback;
        accessory.layoutAttribute = NSLayoutAttributeLeft;
        NSView *view = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 310, 36)];
        NSSegmentedControl *selector = [[NSSegmentedControl alloc] initWithFrame:NSMakeRect(0, 3, 310, 30)];
        selector.segmentCount = 3;
        [selector setLabel:@"SQL" forSegment:0];
        [selector setLabel:@"Shell" forSegment:1];
        [selector setLabel:@"Note" forSegment:2];
        [selector setWidth:100 forSegment:0];
        [selector setWidth:100 forSegment:1];
        [selector setWidth:100 forSegment:2];
        selector.font = [NSFont boldSystemFontOfSize:15];
        selector.segmentStyle = NSSegmentStyleRounded;
        selector.selectedSegment = 0;
        selector.target = accessory;
        selector.action = @selector(selectWorkspace:);
        selector.accessibilityLabel = @"SQL / Shell / Note 工作区";
        accessory.selector = selector;
        [view addSubview:selector];
        accessory.view = view;
        window.titleVisibility = NSWindowTitleHidden;
        [window addTitlebarAccessoryViewController:accessory];
        objc_setAssociatedObject(window, &NaviWorkspaceKey, accessory, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        [selector release];
        [view release];
        [accessory release];
        navi_utilities_install(pointer, callback);
        installed = 1;
    });
    return installed;
}

static NSColor *NaviColor(uint32_t rgb) {
    return [NSColor colorWithSRGBRed:((rgb >> 16) & 255)/255.0
        green:((rgb >> 8) & 255)/255.0 blue:(rgb & 255)/255.0 alpha:1];
}

void navi_workspace_select(uintptr_t pointer, int mode, int dark, uint32_t background, uint32_t accent) {
    NaviOnMain(^{
        NaviWorkspaceTitlebar *accessory = objc_getAssociatedObject((NSWindow *)pointer, &NaviWorkspaceKey);
        if (!accessory) return;
        accessory.selector.selectedSegment = mode;
        NSWindow *window = (NSWindow *)pointer;
        window.titlebarAppearsTransparent = YES;
        window.appearance = [NSAppearance appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
        window.backgroundColor = NaviColor(background);
        accessory.selector.selectedSegmentBezelColor = NaviColor(accent);
        navi_utilities_appearance(pointer, dark);
    });
}

void navi_workspace_remove(uintptr_t pointer) {
    NaviOnMain(^{
        NSWindow *window = (NSWindow *)pointer;
        NaviWorkspaceTitlebar *accessory = objc_getAssociatedObject(window, &NaviWorkspaceKey);
        if (!accessory) return;
        accessory.callback = 0;
        accessory.selector.target = nil;
        navi_utilities_remove(pointer);
        NSUInteger index = [window.titlebarAccessoryViewControllers indexOfObject:accessory];
        if (index != NSNotFound) [window removeTitlebarAccessoryViewControllerAtIndex:index];
        objc_setAssociatedObject(window, &NaviWorkspaceKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        window.titleVisibility = NSWindowTitleVisible;
    });
}
