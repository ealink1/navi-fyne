//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import <stdint.h>

extern void naviUtilityChanged(uintptr_t callback, int action);
static const char NaviUtilitiesKey;

@interface NaviWorkspaceUtilities : NSTitlebarAccessoryViewController
@property(nonatomic, assign) uintptr_t callback;
@property(nonatomic, retain) NSArray<NSButton *> *buttons;
@end

@implementation NaviWorkspaceUtilities
- (void)activate:(NSButton *)sender {
    if (self.callback != 0) naviUtilityChanged(self.callback, (int)sender.tag);
}
- (void)dealloc {
    [_buttons release];
    [super dealloc];
}
@end

static void NaviUtilitiesOnMain(void (^run)(void)) {
    if ([NSThread isMainThread]) run();
    else dispatch_sync(dispatch_get_main_queue(), run);
}

void navi_utilities_install(uintptr_t pointer, uintptr_t callback) {
    NaviUtilitiesOnMain(^{
        NSWindow *window = (NSWindow *)pointer;
        if (!window || objc_getAssociatedObject(window, &NaviUtilitiesKey)) return;
        NaviWorkspaceUtilities *accessory = [[NaviWorkspaceUtilities alloc] init];
        accessory.callback = callback;
        accessory.layoutAttribute = NSLayoutAttributeRight;
        NSView *view = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 116, 36)];
        NSMutableArray<NSButton *> *buttons = [NSMutableArray array];
        NSArray<NSString *> *symbols = @[@"moon", @"lightbulb", @"gearshape"];
        NSArray<NSString *> *labels = @[@"切换到夜间模式", @"AI 助手", @"设置"];
        for (NSInteger i = 0; i < 3; i++) {
            NSButton *button = [[NSButton alloc] initWithFrame:NSMakeRect(4 + i * 36, 3, 32, 30)];
            button.bordered = NO;
            button.imagePosition = NSImageOnly;
            button.image = [NSImage imageWithSystemSymbolName:symbols[i] accessibilityDescription:labels[i]];
            button.toolTip = labels[i];
            button.accessibilityLabel = labels[i];
            button.accessibilityRole = NSAccessibilityButtonRole;
            button.tag = i;
            button.target = accessory;
            button.action = @selector(activate:);
            [view addSubview:button];
            [buttons addObject:button];
            [button release];
        }
        accessory.buttons = buttons;
        accessory.view = view;
        [window addTitlebarAccessoryViewController:accessory];
        objc_setAssociatedObject(window, &NaviUtilitiesKey, accessory, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        [view release];
        [accessory release];
    });
}

void navi_utilities_appearance(uintptr_t pointer, int dark) {
    NaviUtilitiesOnMain(^{
        NaviWorkspaceUtilities *accessory = objc_getAssociatedObject((NSWindow *)pointer, &NaviUtilitiesKey);
        if (!accessory) return;
        NSButton *button = accessory.buttons[0];
        NSString *label = dark ? @"切换到日间模式" : @"切换到夜间模式";
        button.image = [NSImage imageWithSystemSymbolName:dark ? @"sun.max" : @"moon" accessibilityDescription:label];
        button.toolTip = label;
        button.accessibilityLabel = label;
    });
}

void navi_utilities_remove(uintptr_t pointer) {
    NaviUtilitiesOnMain(^{
        NSWindow *window = (NSWindow *)pointer;
        NaviWorkspaceUtilities *accessory = objc_getAssociatedObject(window, &NaviUtilitiesKey);
        if (!accessory) return;
        accessory.callback = 0;
        for (NSButton *button in accessory.buttons) button.target = nil;
        NSUInteger index = [window.titlebarAccessoryViewControllers indexOfObject:accessory];
        if (index != NSNotFound) [window removeTitlebarAccessoryViewControllerAtIndex:index];
        objc_setAssociatedObject(window, &NaviUtilitiesKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    });
}
