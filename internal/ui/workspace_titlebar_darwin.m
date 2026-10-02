//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import <stdint.h>

extern void naviWorkspaceChanged(uintptr_t callback, int mode);
extern void naviAppearanceChanged(uintptr_t callback);
static const char NaviWorkspaceKey;

@interface NaviWorkspaceTitlebar : NSTitlebarAccessoryViewController
@property(nonatomic, assign) uintptr_t callback;
@property(nonatomic, retain) NSSegmentedControl *selector;
@property(nonatomic, retain) NSButton *appearanceButton;
@end

@implementation NaviWorkspaceTitlebar
- (void)selectWorkspace:(NSSegmentedControl *)sender {
    if (self.callback != 0) {
        naviWorkspaceChanged(self.callback, (int)sender.selectedSegment);
    }
}
- (void)toggleAppearance:(NSButton *)sender {
    if (self.callback != 0) naviAppearanceChanged(self.callback);
}
- (void)dealloc {
    [_selector release];
    [_appearanceButton release];
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
        NSView *view = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 266, 36)];
        NSButton *appearance = [[NSButton alloc] initWithFrame:NSMakeRect(4, 3, 32, 30)];
        appearance.bordered = NO;
        appearance.imagePosition = NSImageOnly;
        appearance.target = accessory;
        appearance.action = @selector(toggleAppearance:);
        appearance.accessibilityRole = NSAccessibilityButtonRole;
        accessory.appearanceButton = appearance;
        [view addSubview:appearance];
        NSSegmentedControl *selector = [[NSSegmentedControl alloc] initWithFrame:NSMakeRect(48, 3, 206, 30)];
        selector.segmentCount = 2;
        [selector setLabel:@"SQL" forSegment:0];
        [selector setLabel:@"Shell" forSegment:1];
        [selector setWidth:100 forSegment:0];
        [selector setWidth:100 forSegment:1];
        selector.font = [NSFont boldSystemFontOfSize:15];
        selector.segmentStyle = NSSegmentStyleRounded;
        selector.selectedSegment = 0;
        selector.target = accessory;
        selector.action = @selector(selectWorkspace:);
        selector.accessibilityLabel = @"SQL / Shell 工作区";
        accessory.selector = selector;
        [view addSubview:selector];
        accessory.view = view;
        window.titleVisibility = NSWindowTitleHidden;
        [window addTitlebarAccessoryViewController:accessory];
        objc_setAssociatedObject(window, &NaviWorkspaceKey, accessory, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        [selector release];
        [appearance release];
        [view release];
        [accessory release];
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
        NSString *label = dark ? @"切换到日间模式" : @"切换到夜间模式";
        accessory.appearanceButton.image = [NSImage imageWithSystemSymbolName:dark ? @"sun.max" : @"moon" accessibilityDescription:label];
        accessory.appearanceButton.toolTip = label;
        accessory.appearanceButton.accessibilityLabel = label;
    });
}

void navi_workspace_remove(uintptr_t pointer) {
    NaviOnMain(^{
        NSWindow *window = (NSWindow *)pointer;
        NaviWorkspaceTitlebar *accessory = objc_getAssociatedObject(window, &NaviWorkspaceKey);
        if (!accessory) return;
        accessory.callback = 0;
        accessory.selector.target = nil;
        accessory.appearanceButton.target = nil;
        NSUInteger index = [window.titlebarAccessoryViewControllers indexOfObject:accessory];
        if (index != NSNotFound) [window removeTitlebarAccessoryViewControllerAtIndex:index];
        objc_setAssociatedObject(window, &NaviWorkspaceKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        window.titleVisibility = NSWindowTitleVisible;
    });
}
