# Windows 清理工具市场覆盖面与 Foal 差异

Status: researched 2026-07-17. 本文只使用产品官方文档、官方支持页面或
官方源码。它是市场能力清单和差异输入，不代表 Foal 的实现优先级，也不改变
现有 Clean 删除策略。

## 范围与比较口径

- 必看：Windows Storage Sense / Disk Cleanup、BleachBit、CCleaner。
- 补充：Wise Disk Cleaner、Glary Utilities；只用于验证主流第三方工具的共同覆盖面。
- “Foal 已覆盖”以当前 canonical category catalog 和已实现命令边界为准：
  [`internal/clean/category_catalog.go`](../../internal/clean/category_catalog.go)、
  [`docs/plan/clean-deletion-policy.md`](../plan/clean-deletion-policy.md)。
- “未覆盖”只表示当前没有等价的可执行 Clean category；不等于应当加入。隐私痕迹、
  用户文件、系统级维护和安全擦除与 Foal 的安全边界明显不同。

## 各工具清理内容

### Microsoft Storage Sense、Cleanup recommendations 与 Disk Cleanup

| 能力 | 官方确认的清理内容或行为 | 与 Foal 的关系 |
| --- | --- | --- |
| Storage Sense 默认自动清理 | 系统盘上的不必要临时文件；按时间删除回收站内容。Downloads 和云内容默认不动，只有用户显式配置后才处理。来源：[Microsoft — Manage drive space with Storage Sense](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/manage-drive-space-with-storage-sense) | Foal 覆盖当前用户 `user_temp`，但不是 Storage Sense 的完整“临时文件”集合；Foal 不清空回收站，也不管理 Downloads。 |
| 云文件空间回收 | 可按未打开时长把本地云文件变成 online-only；`Always keep on this device` 豁免，且不是删除云端文件。来源：[Microsoft — Manage drive space with Storage Sense](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/manage-drive-space-with-storage-sense) | Foal 无 OneDrive/其他云提供商的脱水能力；这也不是普通文件删除语义。 |
| Cleanup recommendations | 分为 Temporary files、Large or unused files、Files synced to the cloud、Unused apps；系统还会建议 previous Windows installation(s)。来源：[Microsoft — Free up drive space in Windows](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows)、[Microsoft — Storage settings in Windows](https://support.microsoft.com/en-us/windows/experience/storage-filemanagement/storage-settings-in-windows) | Foal `analyze` 能报告目录占用但不做“大文件/不常用文件”建议；`uninstall` 仅预览；旧 Windows 安装和云同步文件未覆盖。 |
| Disk Cleanup 普通项 | 官方说明默认选择 Downloaded Program Files、Temporary Internet Files、Thumbnails；也可选择其他文件类型。来源：[Microsoft — Free up drive space in Windows](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows) | Foal 有严格 allowlist 的 `inet_cache` 和 `explorer_thumbnail_cache`，没有 Downloaded Program Files 的等价 category。Foal 的范围比 Disk Cleanup 更窄、回收站策略也不同。 |
| Disk Cleanup 系统文件 | “Clean up system files”提供额外系统文件类型；Windows Update 场景也由 Disk Cleanup 清理临时系统文件。来源：[Microsoft — Free up drive space in Windows](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows)、[Microsoft — Free up space for Windows updates](https://support.microsoft.com/en-us/windows/deployment/updates-lifecycle/free-up-space-for-windows-updates) | Foal 不自动提权，管理员范围只报告 `administrator_only_caches` permission boundary；没有 Windows Update Cleanup。 |
| Delivery Optimization | Windows 会自动淘汰 Delivery Optimization cache，也允许在 Disk Cleanup 中手动选中 Delivery Optimization Files。来源：[Microsoft — Delivery Optimization in Windows](https://support.microsoft.com/en-us/windows/deployment/updates-lifecycle/delivery-optimization-in-windows) | Foal 没有 Delivery Optimization cache category。 |
| Previous Windows installation | Settings 的 Temporary files 可删除 Previous Windows Installation(s)。来源：[Microsoft — Manage drive space with Storage Sense](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/manage-drive-space-with-storage-sense) | Foal 没有 `Windows.old` / previous installation cleanup；该类内容具有回滚窗口和系统权限语义。 |

### BleachBit

BleachBit 官方把产品定位为磁盘空间与隐私清理器，通用能力包括 cache、cookies、
logs、recent file lists、temporary files；还提供 preview、cookie manager、custom
cleaners 和 wipe free space。来源：[BleachBit official documentation](https://docs.bleachbit.org/)。

| 能力族 | 官方确认的具体内容 | 与 Foal 的关系 |
| --- | --- | --- |
| 浏览器缓存和状态 | Chrome cleaner 可分别清 AI models、cache、cookies、crash reports、form history、history（访问、下载、缩略图）、search engines、session、site data、sync、passwords、site preferences，并可 vacuum 数据库。Firefox 也覆盖 backup、cache、cookies、crash reports、form history、passwords、session、site data、site preferences、URL history、vacuum。来源：[BleachBit Chrome cleaner source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/google_chrome.xml)、[BleachBit Firefox cleaner source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/firefox.xml) | Foal 只清 Chrome/Edge/Firefox 的严格 cache allowlist，并要求浏览器检查前后都 idle；不清 cookie、登录、密码、历史、session、site data、同步状态或数据库。Foal 是空间清理，BleachBit 同时做隐私/状态重置。 |
| Windows Explorer 痕迹 | 清 UserAssist/MRU、打开/保存对话框历史、RecentDocs、Jump Lists、Run history、search history、Shellbags；源码明确警告 recent documents 会重置 Quick Access pinned locations，Shellbags 会重置桌面图标位置。来源：[BleachBit Windows Explorer cleaner source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/windows_explorer.xml) | Foal 没有注册表清理、MRU/Jump List/search history/Shellbags 清理；这类通常空间收益低且改变用户状态。 |
| Explorer 缩略图 | BleachBit 会结束并重启 Explorer 后删除 `thumbcache*.db`。来源：[BleachBit Windows Explorer cleaner source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/windows_explorer.xml) | Foal 已覆盖精确 allowlist 的 thumbnail/icon DB，但明确不停止进程；锁定失败只局部报告。两者执行边界不同。 |
| 第三方应用 | 官方 cleaner 仓库含 Adobe Reader、Discord、Microsoft Office、Slack、VLC、VS Code 等独立 cleaner。来源：[BleachBit official cleaner directory](https://github.com/bleachbit/bleachbit/tree/master/cleaners) | Foal 已覆盖 VS Code/Cursor 和一组开发工具缓存，但没有通用桌面应用 cleaner catalog，例如 Discord、Office、Adobe、VLC、Slack。 |
| 权限和安全擦除 | Windows 下可通过 UAC 以管理员身份清系统日志；普通用户仍可清用户 profile。还支持 overwrite contents / wipe free space。来源：[BleachBit FAQ](https://docs.bleachbit.org/doc/frequently-asked-questions.html)、[BleachBit official documentation](https://docs.bleachbit.org/) | Foal 不自动提权、不停止进程、不宣称 secure erase，也没有 wipe free space；这是明确产品边界，而非普通 category 缺口。 |

### CCleaner

| 能力 | 官方确认的清理内容或行为 | 与 Foal 的关系 |
| --- | --- | --- |
| OS 与应用分组清理 | Custom Clean 允许分别选择操作系统和应用（含浏览器），先 Analyze，再按 app/category 查看文件和体积。来源：[CCleaner — Custom Clean analysis and results](https://support.ccleaner.com/articles/en_US/Master_Article/custom-clean-s-analysis-and-results-function) | Foal 也 preview-first，但 catalog 更小、策略由 category 固定，不提供任意应用规则集。 |
| 浏览器隐私数据 | CCleaner 可清 cookies，并提供 Cookies to Keep / Intelligent Cookie Scan 来保留常用登录；官方也确认 history 类含访问历史、下载历史、最近输入 URL。来源：[CCleaner — Select cookies to clean](https://support.ccleaner.com/articles/en_US/Master_Article/select-cookies-to-clean-with-ccleaner-for-windows)、[CCleaner 5.66 release notes](https://www.ccleaner.com/knowledge/ccleaner-v5-66-7705) | Foal 不处理 cookie、登录和 history；如果未来扩展，这些不能合并进现有 `browser_cache`，因为用户影响与授权语义不同。 |
| 多类系统 cache | 官方列举 Internet、thumbnail、DNS、font、menu order、tray notifications、window size/location caches，并说明默认选择避免普通用户误选不理解的 cache。来源：[CCleaner — What is my cache and why does it need clearing?](https://www.ccleaner.com/knowledge/what-is-my-cache-and-why-does-it-need-clearing) | Foal 有 Internet allowlist、thumbnail 和 GPU shader caches；没有 DNS/font/menu/tray/window-layout cache。后四类更像状态/体验重置，通常不是高价值磁盘回收。 |
| Microsoft Store / 桌面应用缓存 | 官方示例包括 Firefox Store app、VLC album art/temp、Discord GPU/icon/media cache，并称会清临时文件、日志、缓存等。来源：[CCleaner 6.02 application cleaning announcement](https://www.ccleaner.com/knowledge/ccleaner-6-02-now-cleans-netflix-vlc-whatsapp-and-other-microsoft-store-apps) | Foal 没有 UWP/MSIX 应用容器或通用聊天、媒体应用 cache categories。 |

### Wise Disk Cleaner 与 Glary Utilities（补充验证）

| 工具 | 官方确认的覆盖面 | 与 Foal 的关系 |
| --- | --- | --- |
| Wise Disk Cleaner | Common Cleaner 清系统日志、缩略图、Windows Update 文件、浏览器 cache/history/cookies/saved passwords、Adobe/Office 等第三方应用临时文件；官方强烈不建议清浏览器密码和 cookies，并要求先关闭浏览器。来源：[Wise Disk Cleaner manual — Common Cleaner](https://www.wisecleaner.com/help/wisediskcleaner/how-to-use/common-cleaner.html) | 再次验证 Windows Update、系统/应用日志、广泛应用缓存和浏览器隐私状态是市场常见面；Foal 当前仅覆盖其中 cache 子集，并用 fail-closed idle gate 而不是仅提示关闭浏览器。 |
| Wise Disk Cleaner | 产品还列出 backup files、Windows downloaded update files，并提供 Advanced Cleaner、Slimming System 和 disk defrag。来源：[Wise Disk Cleaner user guide](https://www.wisecleaner.com/wise-disk-cleaner-user-guide.html) | Foal 没有备份/系统瘦身/磁盘整理；后两项属于 optimize 或系统维护，不是当前 Clean 范围。 |
| Glary Utilities Disk Cleanup | 官方列出旧临时文件、obsolete installation files、日志、internet history/cache、error reports、offline content/error logs。来源：[Glary Utilities product page](https://www.glarysoft.com/glary-utilities/) | Foal 已有 temp、WER/crash dump、部分 browser cache；未覆盖安装残留、普通日志、浏览历史、offline content。 |
| Glary Utilities 套件 | 另有 registry repair、broken shortcuts、duplicate files、empty folders、uninstall、startup、defrag、Tracks Eraser、file shredder、disk analyzer 等。来源：[Glary Utilities knowledge base](https://www.glarysoft.com/kb/welcome-to-glary-utilities/) | 这些说明“PC maintenance suite”比 Foal 的 Clean 边界宽很多；Foal 当前只在 `analyze`/preview-only `uninstall` 有邻接能力，且没有 registry repair、去重、启动项、碎片整理或 shredder。 |

## Foal 当前覆盖面汇总

当前 executable Clean matrix 已覆盖以下市场常见内容，且往往比同类工具更严格：

- 当前用户临时文件、crash dumps、Windows Error Reporting。
- Explorer thumbnail/icon DB 和有限 INetCache exact allowlist。
- D3D、NVIDIA、AMD、Intel GPU/shader caches。
- Chrome、Edge、Firefox cache（idle-before-and-after inspection）。
- VS Code、Cursor、Visual Studio、JetBrains caches。
- npm、pnpm、Yarn、Go、pip、Cargo、NuGet、Corepack、uv、Bun 等开发缓存。
- Playwright/Puppeteer 浏览器二进制、Electron 下载 cache。
- 独立 `purge` 清显式 roots 下的 allowlisted rebuildable project artifacts。

来源：[`internal/clean/category_catalog.go`](../../internal/clean/category_catalog.go)、
[`docs/plan/clean-deletion-policy.md`](../plan/clean-deletion-policy.md) 和仓库
[`AGENTS.md`](../../AGENTS.md)。

## 市场存在但 Foal 当前没有等价 Clean category

以下按能力族归纳，不作优先级排序。

| 能力族 | 市场证据 | Foal 当前状态 / 边界差异 |
| --- | --- | --- |
| Windows Update Cleanup / downloaded update files | [Microsoft Disk Cleanup / update guidance](https://support.microsoft.com/en-us/windows/deployment/updates-lifecycle/free-up-space-for-windows-updates)、[Wise manual](https://www.wisecleaner.com/help/wisediskcleaner/how-to-use/common-cleaner.html) | 未覆盖；通常涉及系统级路径、权限、服务状态和更新回滚安全。 |
| Delivery Optimization cache | [Microsoft official guidance](https://support.microsoft.com/en-us/windows/deployment/updates-lifecycle/delivery-optimization-in-windows) | 未覆盖；Windows 自身会自动淘汰，但也允许手动清。 |
| Previous Windows installation / obsolete installation files | [Microsoft Storage Sense guidance](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/manage-drive-space-with-storage-sense)、[Glary product page](https://www.glarysoft.com/glary-utilities/) | 未覆盖；可能消除 Windows 回滚能力，不能当普通 temp。 |
| Downloads aging、Recycle Bin emptying、cloud-file dehydration | [Microsoft Storage Sense guidance](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/manage-drive-space-with-storage-sense) | 未覆盖；涉及用户文件、恢复语义或云 provider API，不适合复用现有 Clean 删除动作。 |
| Large/unused files、unused apps、files synced to cloud | [Microsoft Cleanup recommendations](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows) | `analyze`/`uninstall` 只有邻接的只读能力；没有 Clean category。 |
| 广泛系统日志、安装日志和应用日志 | [BleachBit docs](https://docs.bleachbit.org/)、[Wise manual](https://www.wisecleaner.com/help/wisediskcleaner/how-to-use/common-cleaner.html)、[Glary product page](https://www.glarysoft.com/glary-utilities/) | WER/crash dump 已覆盖，普通 Windows/应用日志未覆盖；日志可能是诊断证据。 |
| 浏览器 cookies、history、download/form history、passwords、session、site data、sync/preferences | [BleachBit Chrome source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/google_chrome.xml)、[CCleaner cookie guidance](https://support.ccleaner.com/articles/en_US/Master_Article/select-cookies-to-clean-with-ccleaner-for-windows) | 刻意只清 cache；这些是隐私/状态清理，可能登出、丢会话或重置设置，必须与 `browser_cache` 分离。 |
| Windows MRU、RecentDocs、Jump Lists、Run/search history、Shellbags | [BleachBit Windows Explorer source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/windows_explorer.xml) | 未覆盖；大多是低空间收益的隐私/体验状态，且部分会重置 Quick Access、桌面图标位置。 |
| DNS/font/menu/tray/window-layout caches | [CCleaner official cache list](https://www.ccleaner.com/knowledge/what-is-my-cache-and-why-does-it-need-clearing) | 未覆盖；DNS 与部分 cache 是低体积，menu/tray/window 项会改变交互状态。 |
| 通用桌面/UWP 应用 caches（Office、Adobe、Discord、Slack、VLC、Store apps 等） | [BleachBit cleaner repository](https://github.com/bleachbit/bleachbit/tree/master/cleaners)、[CCleaner app-cleaning announcement](https://www.ccleaner.com/knowledge/ccleaner-6-02-now-cleans-netflix-vlc-whatsapp-and-other-microsoft-store-apps) | Foal 只覆盖少数开发应用；没有可扩展的 consumer-app cleaner catalog。 |
| Duplicate files、empty folders、invalid shortcuts、registry cleanup | [Glary Utilities knowledge base](https://www.glarysoft.com/kb/welcome-to-glary-utilities/) | 未覆盖；这些不是“确认可再生 cache”，误删/误修风险和验证模型不同。 |
| Secure deletion、file shredding、wipe free space | [BleachBit docs](https://docs.bleachbit.org/)、[Glary Utilities knowledge base](https://www.glarysoft.com/kb/welcome-to-glary-utilities/) | 明确不属于 Foal 当前永久删除语义；Foal 不作 secure-erasure claim。 |
| 自动提权、停止/重启进程 | [BleachBit FAQ](https://docs.bleachbit.org/doc/frequently-asked-questions.html)、[BleachBit Explorer source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/windows_explorer.xml) | Foal 明确不自动提权、不停止应用；权限/锁冲突应 fail closed 或局部跳过。 |

## 对 Foal 的建议优先级

### P0：扩展现有 `browser_cache` 的浏览器覆盖

Foal 当前只注册 Chrome、Edge、Firefox。BleachBit 当前官方资料已经确认覆盖或增强
Brave、Opera、Vivaldi 等 Chromium 浏览器，说明“更多浏览器、仍只清可再生 cache”是
市场上真实且与 Foal 边界一致的缺口。来源：[BleachBit official releases](https://github.com/bleachbit/bleachbit/releases)。

建议保持单一 `browser_cache` category，逐个浏览器完成官方路径/配置证据、profile catalog
验证、精确 cache allowlist 和独立进程 idle gate；不要把 cookies、history、session 或整个
User Data 根目录纳入。现有 [`browserCacheConfigs`](../../internal/clean/browser_cache.go)
已经提供自然扩展点，但不能假设所有 Chromium fork 的目录和进程名完全相同。

### P1：优先研究 CLI agent 本地工件

BleachBit 6.0.2 官方新增 VS Code、Codium、Antigravity、Cursor、Windsurf、Devin、
Claude Code cleaners；Foal 目前只有 VS Code 和 Cursor。来源：
[BleachBit 6.0.2 release notes](https://www.bleachbit.org/news/bleachbit-602)。

Foal 先研究 Claude Code、Codex CLI、Grok Build、Antigravity CLI、Gemini CLI 的 Windows
本地工件，而不是先扩展 VS Code 系桌面编辑器。研究起点必须是完整 inventory 和状态分类，不能预先把 agent home
或 cache-like 目录视为 cache：CLI agent 本地数据可能混合登录凭据、配置、conversation / session
history、项目状态、诊断日志、下载组件和真正可再生 cache。只有逐项证明不含用户状态且可重建
或重新下载的精确子目录，才有资格进入后续 cleanup category 设计。

CLI agent 不复用现有 `applicationCachePolicies`：该 seam 专门表示标准 Roaming AppData 下、
有桌面进程 idle-before-and-after gate 的非浏览器应用缓存。CLI agent 需要先决定运行时并发
边界、全局与项目本地数据边界、history/credential 排除方式，以及是否存在可辨识的完整下载工件。

### P2：改善系统级空间机会的只读交接，而不是自己删除

Windows Update Cleanup、Delivery Optimization 和 previous Windows installation 可能有较大
空间收益，但都落在现有 `administrator_only_caches` / 无自动提权边界之外。建议最多把当前
笼统 permission-boundary notice 改成明确、path-free 的 Windows 内置入口建议，例如 Storage
Temporary files / Cleanup recommendations；不读取受保护目录、不伪造 byte totals、不调用
`cleanmgr`、不自动提权。是否进入主 Clean TUI 需要单独确认，因为当前 TUI 有意只展示
canonical cleanup categories。

### 暂不做

- consumer-app 大目录表：只有在某个应用有高价值、官方可证明的可再生 cache 时逐个讨论；
  不引入 CCleaner/BleachBit 式无限 rule catalog。
- 浏览隐私、Windows 痕迹、普通日志：空间收益与状态破坏不匹配，继续与 Clean 分离。
- Downloads/云文件/大文件/重复文件：属于用户内容管理或分析，不是安全 cache cleanup。
- 注册表、secure erase、wipe free space、清空回收站、进程停止、自动提权：继续明确排除。

## 从市场对照可直接确认的产品形状差异

1. **Foal 的开发者缓存覆盖反而比通用清理器更深。** 当前有 package/build caches、
   browser binaries、Electron、JetBrains、Visual Studio 等结构化 category；第三方官方
   文档通常只笼统写 application temporary files/caches。Foal catalog 证据：
   [`internal/clean/category_catalog.go`](../../internal/clean/category_catalog.go)。
2. **Foal 最大的内容差异集中在 Windows 系统维护面和 consumer-app 面。** 市场共同出现
   Windows Update/installation remnants/system logs，以及 Office/Adobe/Discord/VLC 等
   应用缓存。来源：[Microsoft](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows)、
   [Wise](https://www.wisecleaner.com/help/wisediskcleaner/how-to-use/common-cleaner.html)、
   [BleachBit](https://github.com/bleachbit/bleachbit/tree/master/cleaners)、
   [CCleaner](https://www.ccleaner.com/knowledge/ccleaner-6-02-now-cleans-netflix-vlc-whatsapp-and-other-microsoft-store-apps)。
3. **不少“市场缺口”其实是目标不同。** BleachBit/CCleaner/Wise/Glary 同时承担隐私
   擦除、注册表、优化或完整 PC maintenance；Foal 是 Windows-native、preview-first、
   JSON-first 的安全磁盘清理 CLI。不能用功能数量直接判定 Foal 缺失。边界来源：
   [`AGENTS.md`](../../AGENTS.md)、[BleachBit docs](https://docs.bleachbit.org/)、
   [Glary Utilities KB](https://www.glarysoft.com/kb/welcome-to-glary-utilities/)。
4. **同名内容也可能执行边界不同。** 例如 BleachBit 为 thumbnail cache 强制结束并重启
   Explorer；Foal 不停止进程。Wise 仅要求用户先关闭浏览器；Foal 做检查前后 idle gate。
   来源：[BleachBit Explorer source](https://github.com/bleachbit/bleachbit/blob/master/cleaners/windows_explorer.xml)、
   [Wise Common Cleaner](https://www.wisecleaner.com/help/wisediskcleaner/how-to-use/common-cleaner.html)、
   [`internal/clean/category_catalog.go`](../../internal/clean/category_catalog.go)。

## 资料局限

- 商业工具通常不公开完整内置 rule/path database；本文只记录官方公开能确认的能力，
  不从第三方测评或社区路径列表推断隐藏规则。
- Microsoft 的 Disk Cleanup 支持页公开默认项与系统文件入口，但不保证列出每个 Windows
  build 的全部 handler；本文没有把未在官方页面列出的 UI 项当作已确认事实。
- 工具宣传页中的“安全”“优化性能”等营销表述没有被当作 Foal 产品事实或收益证据。
