# Navi Fyne 功能与界面对齐自检

日期：2026-10-02。环境：macOS arm64、Go 1.26.3、Fyne 2.8.1。
目标目录：`/Users/bre/workspace/self/navi-fyne`。GoNavi 源码目录保持未改动。
**本报告记录已实施和已验证部分；不代表完整的一比一复刻完成。**
旧版 Alpha 的真实服务和原生更新测试保留在[2026-10-01 报告](selfcheck-2026-10-01.md)，
未将这些历史结果重新写成本轮最新 UI 的验收。

## 本轮行为

- 主窗口、工具栏、两行工作标签、绿色主题及上游图标重新对齐。
- 惰性连接树和 SQL 表 / 视图类别、数据库 / schema 选择、独立表数据页。
- 表字段元数据、服务端分页 / 筛选 / 排序、暂存增删改、SQL 预览及受保护提交。
- 字段 / 索引设计；索引新增预览、取消新增、撤销删除；保存前检查结构变化。
- 原生高亮编辑器、行号、Cmd/Ctrl+R、日志 / 参数 / 结果圆角标签。
- 参数化执行和 typed Agent 传输；BLOB 保真、精确整数、预算与缺参数检查。
- 已存查询命名 / 打开 / 删除 / 冲突检查，SQL 文件打开和原子导出；草稿保留库 / schema。
- XLSX / CSV / JSON / Markdown / HTML / INSERT SQL 导出；文件预览、映射与分批导入。
- 全连接只读及数据 / 结构 / 脚本 / 导入独立策略；未知写入结果不重试。

## 查出的缺陷与修复

| 缺陷 | 修复与回归 |
| --- | --- |
| SQLite 固定会话占用唯一连接，元数据访问连接池可能自等待 | 通过同一 session 读取字段、索引、外键、触发器、DDL；真实 `:memory:` 5 秒超时用例 |
| binary 被展示字符串归一化，导致普通字段编辑无法准确定位原行 | 保留 binary 类型并使用独立 cell 元数据传输；原值按列类型绑定；SQLite / DuckDB 编辑含 BLOB 行并验证原 bytes |
| 有参数的 Agent 查询漏传预览预算 | request / scanner 使用与无参数相同预算；10,002 行查询只返回 100 行且标记截断 |
| 输入框失焦或虚拟行回收可能丢失编辑 | 失焦暂存，按 cell identity 保留草稿；无效输入阻止分页 / 提交；回归中文输入、NULL、不同行复用 |
| 已确认回滚仍关闭内存数据库会话 | `RolledBackError` 明确区分成功回滚与未知提交；应用保留确认回滚会话，未知状态关闭；真实约束失败整批回滚 |
| 两个命名查询编辑器可能覆盖保存内容 | 保存 / 删除比较 revision；打开独立草稿，不自动执行；迁移及连接删除级联测试 |
| 对象菜单可能跟随全局连接选择 | 动作绑定被点击节点及 literal schema / name；切换选择后仍打开正确连接、特殊表名不被拆分 |
| Fyne 测试驱动后台内联回调与测试渲染竞争 | 测试使用串行 UI 队列并等待 I/O / 回调都完成；生产默认仍 `fyne.Do`；首次 race 失败后修复，复跑通过 |
| 原生 SelectEntry 更新动作对象导致箭头失去布局 | 保留既有 Button identity，替换回调；数据库选项点击同时重载 schema |
| 切换深色后标签底色及表头前景仍是旧主题 | 软件截图对照发现；自定义 renderer 刷新颜色，明暗往返测试和重新截图 |
| PostgreSQL DDL 占位注释被当作可用元数据 | 明确返回不可用警告；不标为建表脚本或备份 |

## 本机已通过

```sh
python3 tools/check-architecture.py
go run ./tools/check-go-size
python3 tools/verify-upstream.py
python3 tools/verify-ui-assets.py
go test ./...
go test -tags gonavi_full_drivers ./internal/upstream/db ./cmd/driver-agent
go test -race ./internal/application ./internal/infra/... ./internal/domain ./internal/ui ./cmd/release-sign
go vet ./...
NAVIFYNE_TEST_DRIVERS="$PWD/bin/drivers" go test -tags integration -count=1 ./internal/infra/runtime -run 'TestLocalFileAgents|TestLocalAgentsBoundValuesAndOptimisticChanges|TestSQLiteAgentSessionMetadataDoesNotWaitForItsOwnPool'
```

- 架构检查：独立 Fyne，应用 / 领域层不引入 Fyne 或 Wails，无 JVM 运行依赖。
- 体积检查：新生产文件 ≤ 800 行、函数体 ≤ 120 行、测试文件 ≤ 1,500 行。
- 保留上游 524 份文件的来源和哈希通过；97 份图标（84 SVG、13 PNG）哈希 / 清单通过。
- 本轮实际构建 / metadata 握手覆盖全部 22 个可选 Agent，修订前缀 `fyne-values1-`。
- 164 个链接模块及字体 / 图标归属重新生成；Dameng 模块缺根许可证文本的既有分发审核状态保留。
- 真实本地 Agent 回归用独立临时库，不使用用户业务数据；包括文件、内存、metadata、参数、分页预算、binary 编辑及约束回滚。

最终构建命令 `python3 tools/build.py --all-drivers --package` 已通过。
生成 22 个独立 Agent 和 1 个 macOS arm64 应用 ZIP；逐个校验 manifest 的长度、
SHA256、修订及协议，ZIP CRC / 路径检查通过。包内主程序和 Helper 与 `bin/`
对应二进制一致，SQLite 离线 Agent 的 bundle 哈希一致，6 份字体等资源许可文本
以及根归属文档已入包。没有正式签名 / 公证或上传 Release。

| 产物 | 本机路径 / 校验 |
| --- | --- |
| 最新原生应用 | `bin/NaviFyne.app`，v0.1.0，`io.github.ealink1.navifyne` |
| ZIP | `dist/navi-fyne_0.1.0_darwin_arm64.zip`，75.88 MiB |
| ZIP SHA256 | `bdb10d63000b80ce9cedebc0b18132220ccaf24a9342c1475ccfdbe336a1c3c7` |
| 全部产物清单 | `dist/assets-darwin-arm64.json`，23 项，无重复文件名 |

最后的颜色刷新修复后重新运行了完整 UI 单测、UI race 和 `go vet ./...`，均通过；
其他包本轮完整 / race 结果如上。构建后未再修改应用生产代码。

## 视觉自检

已生成并查看浅色查询、深色查询、表数据和字段设计器的 **Fyne 软件渲染截图**。
本地路径及具体对照见[design-qa.md](../design-qa.md)，截图元数据见
`selfcheck-parity-images.json`。软件截图不构成真实 macOS 输入法、鼠标 / 键盘或窗口验收。

电脑控制报告 Mac 锁定；解锁请求尚未收到答复，最新构建的原生复查受阻。
本轮未重启正常使用的应用，未丢弃其连接 / 查询状态，未操作用户业务库写入。
未进行浏览器测试、Git 提交、推送、线上发布或云端 CI 运行。

## 仍未完成

完整对照见[功能矩阵](gonavi-ui-observation-2026-10-01.md#5-与当前-navi-fyne-的逐项差距2026-10-02-更新)。
尤其包括格式化 / 补全 / 执行计划、完整对象类别与右键动作、列管理 / ER、
外键 / 触发器编辑、事务工作台、数据库编辑快照 / 诊断、完整文件向导和任务历史、
比较 / 同步 / 调度 / 持续同步、AI / MCP / Skills、云备份与完整设置中心。

全量参数 / 多结果导出和流式 binary 保真未实现；其他数据库的新分页 / 编辑 / 结构
行为未全部真实测试。Windows / Linux、本机原生 IME / 高负载、正式签名公钥 / Release
及各平台生产安装仍待验收。不能因单测、软件截图或 Agent metadata 通过宣称一比一完成。
