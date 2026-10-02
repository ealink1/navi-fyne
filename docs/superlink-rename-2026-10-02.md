# SuperLink 更名自检

日期：2026-10-02。版本：0.1.0。目标仓库：`ealink1/navi-fyne`。

窗口标题、macOS 应用菜单、欢迎页、关于弹窗、SQL 更新设置、Shell 版本卡片和构建元数据统一为 **SuperLink**。macOS 应用包为 `bin/SuperLink.app`，Linux / Windows Portable 构建目录为 `bin/SuperLink`。启动与实例锁错误提示、内存测量工具及当前操作文档同步更新。

应用 ID 继续使用 `io.github.ealink1.navifyne`，默认数据目录继续使用 `os.UserConfigDir()/NaviFyne`。连接、加密凭据、草稿和设置无需迁移。Go module、仓库、命令行二进制、环境变量及 Release 资产 ID / 文件名保持兼容；GoNavi 上游来源与许可署名保留。

## 验证

- `go test ./internal/ui ./internal/infra/instance ./internal/infra/update` 通过；Go 格式、Python 语法、分层和源码体积检查通过。
- `python3 tools/build.py --all-drivers --package` 成功构建全部 22 个可选 Agent、主程序、Helper 和新应用包。
- 23 项产物的长度 / SHA256、应用包名称 / ID、ZIP CRC / 根目录 / 包内外字节一致性、离线 SQLite 摘要通过。
- `python3 tools/native-smoke.py --upgrade` 在隔离工作区验证真实事件循环、离线 SQLite、实例独占和正常退出；0.1.0 → 0.1.1 整包更新、健康握手、应用 / 状态备份通过。日常应用版本仍为 0.1.0。
- 正常退出旧应用后，用电脑控制打开 `SuperLink.app`。原生窗口与 macOS 菜单显示 SuperLink，欢迎页和关于弹窗实际显示新名称；已有连接及两个查询草稿恢复，未执行其 SQL。
- 当前日常应用从 `bin/SuperLink.app/Contents/MacOS/navi-fyne` 运行；旧应用包保留在忽略的 `.cache/superlink-rename/previous-NaviFyne.app`。

原生截图记录在 `.cache/superlink-rename/welcome.png`、`about.png`；产物校验记录在同目录 `artifacts.json`。未进行浏览器测试、Git 提交或推送。

## 当前产物

| 项目 | 值 |
| --- | --- |
| 主程序 SHA256 | `ca396ec8fec38143507a89edfda14b51f5d286658e6b9abf5d58fffc9792242a` |
| 应用 ZIP | `dist/navi-fyne_0.1.0_darwin_arm64.zip`，根目录 `SuperLink.app` |
| ZIP 长度 | 87,555,924 字节 |
| ZIP SHA256 | `7344a138e40f485aa9460c023e74cb7e0e7ef5c598fdbc5ebe2addcac747f27e` |

本次原生运行验收为 macOS arm64；其他平台仅更新构建名称，尚未在对应系统上运行验收。
