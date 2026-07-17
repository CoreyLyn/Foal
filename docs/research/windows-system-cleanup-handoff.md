# Windows 系统清理交接的官方依据

Status: researched 2026-07-17. 本文只使用微软官方支持文档和 Microsoft Learn。
它为 Foal P2 的只读、path-free 交接文案提供事实依据，不设计扫描、执行或删除方案。

## 结论

Foal 应把这类机会统称为 **Windows-managed system storage**，固定说明：

- `Windows-managed`
- `Not measured by Foal`
- `Not included in Potential space`

最稳妥的共同入口是 `Settings > System > Storage`。后续页面应按版本表达：

- Windows 11：`Cleanup recommendations` 或 `Temporary files`。
- Windows 10：`Temporary files`；需要更多系统项时使用
  `Disk Cleanup > Clean up system files`。不要承诺 Windows 10 一定存在
  `Cleanup recommendations`。

建议英文文案：

> Windows-managed system storage is not measured by Foal. Open Settings >
> System > Storage. On Windows 11, review Cleanup recommendations or Temporary
> files. On Windows 10, review Temporary files or use Disk Cleanup > Clean up
> system files. This space is not included in Potential space.

这段文案只给页面名称，不包含 `SoftwareDistribution`、`Windows.old`、`WinSxS`
或其他内部路径；也不暗示 Foal 已检测到可回收空间。

## Windows 10 与 Windows 11 的页面差异

### `Temporary files`

`Settings > System > Storage > Temporary files` 是两个版本都适用的稳定入口。
微软当前按 Windows 11 / Windows 10 分栏的性能指引，两个分栏都给出该路径及
`Remove files` 操作。来源：
[Microsoft Support — Tips to improve PC performance in Windows](https://support.microsoft.com/en-US/Windows/Experience/Performance-Optimization/tips-to-improve-pc-performance-in-windows)。

因此 P2 可以把 `Temporary files` 作为跨版本的主要页面名称，但不能据此声称页面
一定列出某种特定系统内容；可见项目取决于系统当前状态。

### `Cleanup recommendations`

Windows 11 的明确路径是
`Settings > System > Storage > Cleanup recommendations`。微软列出的建议类型包括
`Temporary files`、`Large or unused files`、`Files synced to the cloud` 和
`Unused apps`。来源：
[Microsoft Support — Free up drive space in Windows](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows)。

微软针对 `Windows.old` 的版本化 KB 明确区分：Windows 10 使用 Storage Sense 的
`Change how we free up space automatically > Free up space now`，Windows 11 使用
`Cleanup recommendations`。来源：
[Microsoft Support — KB5012334](https://support.microsoft.com/en-US/servicing/os/windows/2022/02/kb5012334-delete-the-windows-old-folder-using-storage-sense-in-the-settings-app)。

部分较新的通用支持页同时标注 Windows 10 和 Windows 11，并统一写
`Cleanup recommendations`；但上述版本化 KB 以及微软按版本分栏的旧版指引表明，
Windows 10 的页面形态会因版本而不同。Foal 因此不应把 `Cleanup recommendations`
写成 Windows 10 的必有入口。

### `Disk Cleanup > Clean up system files`

微软仍把 Disk Cleanup 作为删除临时系统文件的内置入口；选择
`Clean up system files` 后，Windows 重新计算可处理的系统文件类型。来源：
[Microsoft Support — Free up drive space in Windows](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/free-up-drive-space-in-windows)、
[Microsoft Support — Free up space for Windows updates](https://support.microsoft.com/en-US/Windows/Deployment/Updates-Lifecycle/free-up-space-for-windows-updates)。

这是 Windows 10 的兼容后备入口，也可在 Windows 11 使用。P2 只应交接到该工具，
不应把它的当前选项、估算或结果归因于 Foal。

## 三类系统管理内容

### Windows Update Cleanup

旧 Windows Update 组件属于 Windows component store 的 servicing 生命周期，不是普通
文件缓存。Windows 会通过内部过程和 `StartComponentCleanup` 任务清理已被新版本替代
的组件；自动任务至少等待 30 天再卸载旧组件，以保留一段回滚窗口。微软明确警告不要
手动删除 `WinSxS` 内容；内置 Disk Cleanup 的 update-cleanup 选项会缩减 component
store。来源：
[Microsoft Learn — Clean Up the WinSxS Folder](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/clean-up-the-winsxs-folder?view=windows-11)。

产品含义：Foal 只能把“旧更新和系统组件”描述为 Windows 管理的空间机会，并交接给
Windows Storage / Disk Cleanup。不要写“可安全删除全部 Windows Update cache”，也不要
暴露或建议直接操作系统内部目录。

### Delivery Optimization Files

Delivery Optimization 为 Windows Update、Microsoft Store 应用和其他 Microsoft 内容
提供下载与可选的对等分发。Windows 会在短时间后或缓存占用过大时自动清理其缓存；
需要立即释放空间时，微软支持用户在 Disk Cleanup 中选择
`Delivery Optimization Files`。来源：
[Microsoft Support — Delivery Optimization in Windows](https://support.microsoft.com/en-us/windows/deployment/updates-lifecycle/delivery-optimization-in-windows)、
[Microsoft Learn — Delivery Optimization reference](https://learn.microsoft.com/en-us/windows/deployment/do/waas-delivery-optimization-reference)。

产品含义：应称为 Windows 自动管理的传递优化缓存，并让用户在 Windows 内置工具中
“查看”或“检查”；不要假设内部路径、大小或组织策略状态。

### Previous Windows installation(s)

微软页面使用过 `Previous Windows installation(s)` 和 `Previous version of Windows`
两种名称。它们指向升级前的 Windows 版本及 `Windows.old`：通常在升级 10 天后由
Windows 自动删除；提前删除需要管理员权限，且不可撤销，会失去回退到先前 Windows
版本的能力。来源：
[Microsoft Support — Delete your previous version of Windows](https://support.microsoft.com/en-US/Windows/Deployment/Install-Upgrade/delete-your-previous-version-of-windows)、
[Microsoft Support — Manage drive space with Storage Sense](https://support.microsoft.com/en-US/Windows/Experience/Storage-FileManagement/manage-drive-space-with-storage-sense)。

产品含义：如果 P2 需要解释潜在收益，应写“以前的 Windows 安装可能占用较多空间；
删除后无法回退，请确认当前系统稳定后通过 Windows 设置处理”。不要把它归类为普通
临时文件，也不要承诺当前机器一定存在该项。

## 对 P2 文案的约束

1. 固定显示共同入口，不依赖 Foal 扫描结果或普通 `permission_denied` 动态触发。
2. 使用“review / 查看 / 检查”，不使用“clean all / 全部清理”；`Cleanup
   recommendations` 也可能包含个人大文件、云同步文件和未使用应用。
3. 只显示 Windows 页面或工具名称，不显示文件系统路径、估算字节或候选数。
4. 不计入 `Potential space`，不进入可选择 category，不写 Result 或 History。
5. 文案不声称 Foal 已测量、验证或授权任何系统级删除。
