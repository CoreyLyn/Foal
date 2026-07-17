# Windows CLI coding agents local artifacts

Status: researched 2026-07-17. 本文只使用产品官方文档与官方源码，未读取
本机用户数据。目标是识别本地数据的语义边界，不代表 Foal 已决定新增清理类别。

## 比较口径

Windows 表格中的 `%USERPROFILE%` 对应官方文档里的 `~`。除非官方明确提供覆盖变量，
本文不推断其他安装方式、旧版本或 IDE/桌面端的路径。

| 分类 | 含义 | 能否仅因“占空间”视为 cache |
| --- | --- | --- |
| 可再生 cache/download | 可由产品重新生成/下载，不承载用户连续性 | 仍需官方证明准确根、边界和重建影响 |
| logs | 诊断或运行日志 | 可能可清，但先确认 retention、写入进程和诊断价值 |
| conversation/session history | prompt、response、tool calls/results、resume 数据 | 否；删除会丢会话连续性，且可能含源码和 secrets |
| credentials/auth | OAuth token、refresh token、API key、MCP token、OS keyring | 绝不能归入 cache；删除会登出或破坏集成 |
| config/installed extensions | settings、rules、skills、plugins、agents、hooks、MCP config | 否；这是用户配置或已安装资产 |
| project/local state | memory、plans、tasks、checkpoints、file snapshots、search index | 不能按目录名一概而论；通常会丢恢复、记忆或任务状态 |

## Claude Code

Claude 官方明确说明 Windows 上 `~/.claude` 解析为 `%USERPROFILE%\.claude`，并可由
`CLAUDE_CONFIG_DIR` 整体改址。来源：[Explore the .claude directory](https://code.claude.com/docs/en/claude-directory)。

### 用户级数据

| Windows 路径（默认） | 分类 | 官方确认的内容 | 清理边界 |
| --- | --- | --- | --- |
| `%USERPROFILE%\.claude\projects\<project>\<session>.jsonl` | conversation/session history | 完整 plaintext transcript：消息、tool call、tool result；默认由 `cleanupPeriodDays` 在启动时清理超过 30 天的数据。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory)、[Manage sessions](https://code.claude.com/docs/en/sessions) | 不是 cache。删除会失去 resume/continue/rewind，且 transcript 可能包含工具读取的 `.env` 内容、命令输出或其他 secrets。 |
| `%USERPROFILE%\.claude\projects\<project>\<session>\tool-results\` | conversation/session history | 大型工具输出从 transcript 外溢到独立文件；同受 `cleanupPeriodDays` 管理。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 不是独立 cache；与所属 session 一起管理，不能孤立按“临时输出”删除。 |
| `%USERPROFILE%\.claude\history.jsonl` | conversation/session history | 所有输入 prompt、时间戳和 project path，用于上箭头召回；不在自动 cleanup 范围。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 不是 cache；删除会失去 prompt recall。 |
| `%USERPROFILE%\.claude\projects\<project>\memory\` | project/local state | Claude 自动保存的项目 build 命令、调试经验、架构/风格/工作流记忆；跨会话加载。来源：[Claude auto memory](https://code.claude.com/docs/en/memory) | 不是 cache；属于持久项目知识。 |
| `%USERPROFILE%\.claude\file-history\<session>\` | project/local state | Claude 编辑文件前的 snapshots，用于 checkpoint restore；同受 `cleanupPeriodDays` 管理。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 删除会失去旧 session checkpoint restore，不是无影响 cache。 |
| `%USERPROFILE%\.claude\plans\`、`tasks\`、`session-env\` | project/local state | Plan files、session task lists、per-session environment metadata；官方列入自动 cleanup 集合。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 仅能按官方 retention/session 生命周期考虑；不能 whole-root 与活跃 session 竞争。 |
| `%USERPROFILE%\.claude\debug\` | logs | 仅 `--debug` 或 `/debug` 时写入的 per-session debug logs；同受自动 cleanup。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 明确是 logs；删除会丢诊断证据。 |
| `%USERPROFILE%\.claude\paste-cache\`、`image-cache\` | 可再生 cache/download，但可能含敏感输入 | 大型 paste 与 attached image 内容；同受自动 cleanup。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 官方名称虽为 cache，但内容来自用户输入，可能敏感；必须避开活跃 session，并尊重内建 retention。 |
| `%USERPROFILE%\.claude\shell-snapshots\` | project/local state | Bash tool 捕获的 shell environment；正常退出时删除，崩溃残留由启动 sweep 清理。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 只有 crash leftovers 接近可清临时物；产品自身已有生命周期清理。 |
| `%USERPROFILE%\.claude\stats-cache.json` | 可重建统计 cache | `/usage` 展示的 aggregated token/cost counts；长期保留。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 文件可删除，但会丢 historical usage totals；不是零影响 cache。 |
| `%USERPROFILE%\.claude\backups\` | config backup | `~/.claude.json` migration 前的 timestamped backups；官方清理表说明不在 project purge 范围。来源：[Claude application data](https://code.claude.com/docs/en/claude-directory) | 不是普通 cache；保留配置恢复价值。 |
| `%USERPROFILE%\.claude.json` | credentials/auth + config + app state | OAuth session、personal/user MCP、per-project trust/allowed tools、UI state和 various caches；Claude 文档明确警告不要删除。来源：[Claude settings](https://code.claude.com/docs/en/settings)、[Claude application data](https://code.claude.com/docs/en/claude-directory) | 混合敏感文件，绝不能作为 cache root。 |
| `%USERPROFILE%\.claude\settings.json`、`CLAUDE.md`、`rules\`、`skills\`、`agents\`、`plugins\` 等 | config/installed extensions | 用户级设置、指令、rules、skills、subagents、plugins。来源：[Explore the .claude directory](https://code.claude.com/docs/en/claude-directory) | 用户创作/安装资产，不清。 |

### 项目内数据

| 路径 | 分类 | 内容 |
| --- | --- | --- |
| `<project>\.claude\settings.json`、`settings.local.json` | config | shared project settings 与个人 project override。来源：[Claude settings](https://code.claude.com/docs/en/settings) |
| `<project>\CLAUDE.md` / `.claude\CLAUDE.md`、`.claude\rules\`、`skills\`、`agents\` | config/project state | 项目指令、rules、skills 与 subagent definitions。来源：[Explore the .claude directory](https://code.claude.com/docs/en/claude-directory) |
| `<project>\.mcp.json` | config，可能含 secret references | team-shared project MCP servers。来源：[Claude settings](https://code.claude.com/docs/en/settings) |

Claude 提供官方 `claude project purge <path> --dry-run`，可预览并按项目删除 transcript、
task/debug/file-history、prompt history lines、auto memory 与 `~/.claude.json` 项目条目。
这比第三方按目录猜测更可靠；它也刻意不碰 `shell-snapshots/` 和 `backups/`。
来源：[Claude application data](https://code.claude.com/docs/en/claude-directory)、
[CLI reference](https://code.claude.com/docs/en/cli-usage)。

## Grok Build

Grok Build 官方源码说明 Windows 预编译版本受支持；默认数据根是 `~/.grok`，可由
`GROK_HOME` 覆盖。Windows 默认因此为 `%USERPROFILE%\.grok`。来源：
[Grok Build official repository](https://github.com/xai-org/grok-build)、
[Grok configuration guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/05-configuration.md)。

### 用户级数据

| Windows 路径（默认） | 分类 | 官方确认的内容 | 清理边界 |
| --- | --- | --- | --- |
| `%USERPROFILE%\.grok\sessions\<encoded-cwd>\<session-id>\` | conversation/session history + project state | 每个 session 含 `summary.json`、authoritative `updates.jsonl`（conversation + tool calls）、`chat_history.jsonl`、`plan.json`、`rewind_points.jsonl`、`signals.json`、`feedback.jsonl`、compaction checkpoints 与 subagent metadata。来源：[Grok session guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/17-sessions.md) | 不是 cache；删除会破坏 resume、search、plan、rewind 和会话审计。 |
| `%USERPROFILE%\.grok\memory\` | project/local state | global/workspace `MEMORY.md`、per-session summaries/logs 与 SQLite hybrid-search index；memory 默认实验性且关闭。来源：[Grok memory guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/13-memory.md) | `index.sqlite`可由 Markdown 重建这一点在文档中只体现为“index supports search”，未明确承诺删除后自动 rebuild；whole memory root 属于持久知识，不清。 |
| `%USERPROFILE%\.grok\logs\` | logs | Internal logs，例如 `unified.jsonl` 和 MCP server logs。来源：[Grok configuration guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/05-configuration.md) | 官方未给 retention、轮转、锁定与安全删除承诺；只能确认是 diagnostics，不能直接断言 whole-root safe。 |
| `%USERPROFILE%\.grok\auth.json` | credentials/auth | 浏览器/OIDC/external auth tokens；自动 refresh，`grok logout` 会清 cached credentials。来源：[Grok authentication guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/02-authentication.md) | 绝不当 cache；删除会登出。 |
| `%USERPROFILE%\.grok\config.toml`、`pager.toml`、`lsp.json` | config | 主配置、TUI appearance、LSP servers。来源：[Grok configuration guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/05-configuration.md) | 用户配置，不清。`config.toml` 还可能包含 per-model API key 或 telemetry collector token。 |
| `%USERPROFILE%\.grok\skills\`、`plugins\`、`agents\` | config/installed extensions | 用户级 skills、plugins、agent definitions。来源：[Grok configuration guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/05-configuration.md) | 已安装或用户创作资产，不按 download cache 清。 |

### 项目内数据

`<project>\.grok\` 可包含 `config.toml`、skills、plugins、agents、hooks、`lsp.json`、
`sandbox.toml`；项目 `AGENTS.md` 也会作为 system instructions。它们全部是项目配置，
不是 cache。来源：[Grok configuration guide](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/05-configuration.md)。

### 官方证据未覆盖

当前官方 file-locations 表没有列出独立的 regenerable download/cache root，配置文档中也
没有 `cache` 项。不能依据安装器、插件来源或社区观察猜测额外目录。官方能明确识别的
潜在空间面只有 logs，以及 sessions/compaction/rewind 等有状态数据；后者不是 cache。

### P1 tracer：官方 updater 源码补证

P1 首先深入 Grok Build。检查官方仓库 2026-07-16 同步提交
[`8adf901`](https://github.com/xai-org/grok-build/tree/8adf9013a0929e5c7f1d4e849492d2387837a28d)
后，`downloads/` 必须拆成不同生命周期，而不能整体清理：

- 内建 updater 把已 smoke-tested 的 versioned binary 下载到
  `~/.grok/downloads/grok-<version>-<platform>`，激活后明确保留 current + one previous；
  新鲜 `.tmp` 还可能属于并发 updater。手工清整个目录在 Unix 会形成 dangling active
  symlink，官方源码也将其列为破坏场景。来源：
  [`auto_update.rs` download/activation](https://github.com/xai-org/grok-build/blob/8adf9013a0929e5c7f1d4e849492d2387837a28d/crates/codegen/xai-grok-update/src/auto_update.rs#L1160-L1239)、
  [`cleanup_old_downloads`](https://github.com/xai-org/grok-build/blob/8adf9013a0929e5c7f1d4e849492d2387837a28d/crates/codegen/xai-grok-update/src/auto_update.rs#L1737-L1856)、
  [`version.rs` dangling-link warning](https://github.com/xai-org/grok-build/blob/8adf9013a0929e5c7f1d4e849492d2387837a28d/crates/codegen/xai-grok-update/src/version.rs#L419-L437)。
- Windows `install.ps1` 还会把一次安装的 staging payload 写到固定
  `downloads/grok-windows-<arch>.exe`，再复制到 `bin/grok.exe` 与 `bin/agent.exe`。
  它不是运行入口，但脚本没有原子 temp/publish 或安装锁，第三方并发清理存在破坏安装
  rollback 的窗口；不能仅按名称直接加入 P1。来源：
  [official `install.ps1`](https://github.com/xai-org/grok-build/blob/8adf9013a0929e5c7f1d4e849492d2387837a28d/crates/codegen/xai-grok-pager/scripts/install.ps1#L188-L230)。
- Windows updater 为替换被锁定的 `bin/grok.exe` / `bin/agent.exe`，会把旧进程镜像移到
  exact `.old` 或 `*.old.<pid>-<seq>.old` sibling。官方每次 update 开始都会 best-effort
  sweep；仍被运行中旧进程锁住的文件留到以后清。这些精确 update residue 是当前证据
  最强的 Grok 候选，但 Foal 仍需决定 idle gate、freshness guard 和 category contract。
  来源：[`windows_replace_exe` and sweep](https://github.com/xai-org/grok-build/blob/8adf9013a0929e5c7f1d4e849492d2387837a28d/crates/codegen/xai-grok-update/src/auto_update.rs#L1614-L1735)。

本机非内容元数据显示：`bin/grok.exe.old` 约 138 MB；固定 staging payload
`downloads/grok-windows-x86_64.exe` 约 138 MB；current versioned download 约 129 MB。
前两者时间戳相同且内容大小相同，但本机观察只用于佐证收益，不单独证明 eligibility。

## Antigravity CLI

Windows installer 将 `agy` binary 放在 `%LOCALAPPDATA%\agy\bin`；应用配置与状态主要
位于 `%USERPROFILE%\.gemini\antigravity-cli`。认证采用 OS native keyring。来源：
[Antigravity CLI getting started](https://antigravity.google/docs/cli-getting-started)、
[official repository](https://github.com/google-antigravity/antigravity-cli)。

### 用户级数据

| Windows 路径（默认） | 分类 | 官方确认的内容 | 清理边界 |
| --- | --- | --- | --- |
| `%USERPROFILE%\.gemini\antigravity-cli\brain\<conversationId>\.system_generated\logs\transcript.jsonl` | conversation/session history | Persistent transcript conversation logs；hooks 会收到其绝对路径。来源：[Antigravity hooks](https://www.antigravity.google/docs/hooks) | 虽位于 `logs` 子目录，但语义是完整 conversation transcript，不是 disposable diagnostic log。 |
| `%USERPROFILE%\.gemini\antigravity-cli\brain\<conversationId>\...` | conversation artifacts/project state | 官方 hooks contract 将同一 conversation root 作为 `artifactDirectoryPath`，含 conversation artifacts 与 screenshots。来源：[Antigravity hooks](https://www.antigravity.google/docs/hooks) | 不是 cache；删除会丢会话产物。官方未公开完整 child allowlist，不能猜局部可清目录。 |
| `%USERPROFILE%\.gemini\antigravity-cli\cache\last_conversations.json` | 可再生 lookup cache | 绝对 workspace path 到最近 conversation ID 的映射；`agy -c` 读取后向 backend 验证。缺失、无效或远端已删除时会开始新 session。来源：[Antigravity resume command](https://antigravity.google/docs/cli/commands/resume) | 当前四款中最明确的纯 lookup cache。删除影响是失去 workspace 的“continue latest”快捷映射，不删除远端 conversation；仍应避开活跃进程。 |
| OS native keyring | credentials/auth | 首次迁移/登录把 session tokens 存入 OS native keyring；`/logout` 清除。来源：[Antigravity official repository](https://github.com/google-antigravity/antigravity-cli)、[migration guide](https://www.antigravity.google/docs/gcli-migration) | 不是文件 cache；Foal 不应枚举或修改 keyring。 |
| `%USERPROFILE%\.gemini\antigravity\mcp_oauth_tokens.json` | credentials/auth | MCP OAuth access tokens；过期自动 refresh、无效会移除。注意官方路径属于 shared Antigravity auth state，不在 `antigravity-cli` 子目录。来源：[Antigravity MCP guide](https://antigravity.google/docs/mcp) | 绝不清；会断开 MCP 登录。 |
| `%USERPROFILE%\.gemini\antigravity-cli\settings.json`、`keybindings.json` | config | Sparse user preferences、permissions、sandbox、appearance、key maps。来源：[Antigravity settings](https://antigravity.google/docs/cli-settings) | 用户配置，不清。 |
| `%USERPROFILE%\.gemini\config\mcp_config.json` | config，可能直接含 credentials | Global MCP server definitions；schema 允许 env、HTTP Authorization headers、OAuth client secret。来源：[Antigravity MCP guide](https://antigravity.google/docs/mcp) | 敏感配置，不清。 |
| `%USERPROFILE%\.gemini\antigravity-cli\plugins\<plugin_name>\`、`skills\`、`import_manifest.json` | config/installed extensions | Installed plugin bundle 可含 MCP、hooks、skills、agents、rules；manifest 跟踪导入。来源：[Antigravity CLI features](https://antigravity.google/docs/cli-features)、[migration guide](https://www.antigravity.google/docs/gcli-migration) | 已安装资产，不应按 downloaded cache 删除。 |
| `%LOCALAPPDATA%\agy\bin` | installed binary | Windows 默认 installer target。来源：[Antigravity CLI getting started](https://antigravity.google/docs/cli-getting-started) | 安装目录，不是 cache。 |

### 项目内数据

| 路径 | 分类 | 内容 |
| --- | --- | --- |
| `<project>\.agents\mcp_config.json` | config，可能含 credentials | Workspace MCP definitions；支持 env、headers 和 OAuth client fields。来源：[Antigravity MCP guide](https://antigravity.google/docs/mcp) |
| `<project>\.agents\skills\` | config/project state | Workspace-local skills。来源：[migration guide](https://www.antigravity.google/docs/gcli-migration) |
| `<project>\GEMINI.md`、`AGENTS.md` | config/project state | Workspace rules/context，Antigravity CLI 继续读取。来源：[migration guide](https://www.antigravity.google/docs/gcli-migration) |

### 官方证据未覆盖

- 官方文档未公开 conversation root 下所有 task/subagent/artifact 子路径，因此不能制作
  “只删某几个日志文件”的 allowlist。
- 除 `cache/last_conversations.json` 外，没有官方确认的独立下载缓存或模型缓存根。
- `/resume` 会验证 backend conversation，且 transcript 同时存在本地；官方资料没有完整
  说明删除本地 transcript 后各 surface 的恢复一致性，不能假设远端一定可重建全部内容。

## Gemini CLI

Gemini CLI 默认用户根是 `%USERPROFILE%\.gemini`；`GEMINI_CLI_HOME` 可改变用户 home，
CLI 再在其中创建 `.gemini`。来源：[Gemini CLI configuration](https://geminicli.com/docs/get-started/configuration-v1/)。

### 用户级数据

| Windows 路径（默认） | 分类 | 官方确认的内容 | 清理边界 |
| --- | --- | --- | --- |
| `%USERPROFILE%\.gemini\tmp\<project_hash>\chats\` | conversation/session history | 自动保存完整 conversation，包括 prompts、model responses、tool inputs/outputs、token stats、reasoning summaries；project-scoped。来源：[Gemini session management](https://geminicli.com/docs/cli/session-management/) | 目录名是 `tmp` 但内容不是 cache；删除会失去 `/resume` sessions。 |
| `%USERPROFILE%\.gemini\tmp\<project_hash>\` 下 manual chat checkpoints | conversation/session history | `/resume save <tag>` 保存可恢复 conversation checkpoint；Windows 官方路径明确为 `C:\Users\<user>\.gemini\tmp\<project_hash>\`。来源：[Gemini CLI commands](https://geminicli.com/docs/cli/commands/) | 不是 cache；删除会丢手动保存的分支点。 |
| `%USERPROFILE%\.gemini\tmp\<project_hash>\checkpoints\` | project/local state | 开启 checkpointing 后保存 conversation history 与原 tool call；配套 shadow Git snapshot 可恢复项目文件。来源：[Gemini checkpointing](https://geminicli.com/docs/cli/checkpointing/) | 不是 cache；删除会破坏 `/restore` 的会话侧 checkpoint。 |
| `%USERPROFILE%\.gemini\history\<project_hash>` | project/local state | Checkpointing 的 shadow Git repository，保存修改前 project snapshot。来源：[Gemini checkpointing](https://geminicli.com/docs/cli/checkpointing/) | 不是普通 Git cache；删除会丢项目恢复点。 |
| `.gemini\tmp\tracker\<session-id>`（官方给出相对根） | project/local state | Tracker tools 的 per-session task state。来源：[Gemini tracker tools](https://geminicli.com/docs/tools/tracker/) | Session state；官方页面未明确它相对 user root还是 workspace root，也未给 retention，不能猜绝对 Windows target。 |
| `%USERPROFILE%\.gemini\settings.json` | config | User settings；project 可有 `<project>\.gemini\settings.json` 覆盖。来源：[Gemini settings](https://geminicli.com/docs/cli/settings/) | 用户配置，不清。 |
| `%USERPROFILE%\.gemini\commands\`、`skills\`、extensions | config/installed extensions | User custom commands/skills/extensions；commands 可从 user、project 与 active extensions 加载。来源：[Gemini CLI command reference](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/commands.md)、[Gemini configuration](https://geminicli.com/docs/get-started/configuration-v1/) | 用户创作或安装资产，不清。 |
| Cached Google sign-in credential，或 env/API key/service-account path | credentials/auth | Headless mode会复用 existing cached credential；其他方式使用 `GEMINI_API_KEY`、`GOOGLE_APPLICATION_CREDENTIALS` 或 ADC。来源：[Gemini authentication](https://geminicli.com/docs/get-started/authentication/) | 官方公开认证页没有承诺 cached OAuth credential 的具体 on-disk 文件路径。不要从社区帖子推断 `oauth_creds.json` 并据此清理。 |

### 项目内数据

| 路径 | 分类 | 内容 |
| --- | --- | --- |
| `<project>\.gemini\settings.json` | config | Project settings，覆盖 user settings。来源：[Gemini settings](https://geminicli.com/docs/cli/settings/) |
| `<project>\.gemini\commands\`、`skills\` | config/project state | Project custom commands 与 skills。来源：[Gemini command reference](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/commands.md) |
| `<project>\GEMINI.md` | config/project state | Hierarchical instructional context。来源：[Gemini command reference](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/commands.md) |

### 官方证据未覆盖

- 官方文档没有确认独立、纯可再生的 download/model cache root。
- Session deletion 会连带 implementation plans、task trackers、tool outputs 和 activity logs，
  但官方 session 文档没有发布这些 child paths 的完整 allowlist。来源：
  [Gemini session management](https://geminicli.com/docs/cli/session-management/)。
- 不能因为父目录名是 `tmp` 就清 `%USERPROFILE%\.gemini\tmp`；其中明确保存自动会话、
  手动 chat checkpoints 和 file-restore checkpoints。

## Codex CLI

Codex CLI 的默认状态根是 `$CODEX_HOME`（未设置时通常为 `%USERPROFILE%\.codex`）。
官方 CLI 把 `resume`、`archive`、`delete` 作为已保存会话的生命周期入口；因此会话目录、
索引和状态数据库是用户状态，不是缓存。Codex Desktop 与 CLI 还可能共享同一个
`$CODEX_HOME`，Foal 不能把整个 `.codex` 当作单一产品的临时目录。

| 路径/表面 | 分类 | P1 判断 |
| --- | --- | --- |
| `sessions/`、`archived_sessions/`、`session_index.jsonl`、state DB | history/state | 保存会话、归档和索引；必须排除。删除应走 `codex delete` |
| `auth.json`、`config.toml`、rules/skills/AGENTS、用户安装的 plugins | credentials/config/user assets | 必须排除；插件生命周期应优先走 `codex plugin` |
| `.tmp/bundled-marketplaces/`、`.tmp/marketplaces/`、`.tmp/plugins/`、plugin cache | active product materialization | 名称含 `tmp/cache` 仍不代表垃圾；官方实现会从这些位置发现、刷新 marketplace/plugin，P1 不清 |
| `cache/` | mixed application cache | 本机可见多个 Codex App capability cache，但官方未发布完整 child allowlist 或删除保证；P1 不清 |
| `models_cache.json` | lookup cache | 可推断为模型目录缓存且体积很小，但缺少官方 cleanup contract；P1 不清 |
| `log/`、`logs_2.sqlite*` | diagnostics/runtime state | 可能很大，但官方未给 retention、并发与安全删除契约；P1 不清 |

本机只做了名称、类型、大小和时间戳枚举，没有读取文件正文：`.codex` 中 `sessions/`
约 252 MB、`archived_sessions/` 约 27 MB、plugins 约 467 MB、`.tmp` 约 136 MB，
`logs_2.sqlite` 约 572 MB。大体积主要落在敏感 history/state、活跃插件物化和无公开
清理契约的诊断数据库中，不能用“空间收益高”替代安全证明。

来源：[Codex 官方仓库及 CLI README](https://github.com/openai/codex)、
[app-server marketplace lifecycle](https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md)。
本机安装的官方 `@openai/codex` 0.144.5 `--help` 还确认了 session archive/delete、
plugin remove 与 `$CODEX_HOME/config.toml` 表面。官方仓库中的
[bundled marketplace implementation evidence](https://github.com/openai/codex/issues/21936) 与
[plugin cache implementation evidence](https://github.com/openai/codex/issues/23902)
只作为当前实现证据，不视为稳定清理契约。

## 跨产品对照

| 产品 | 官方明确的可再生/lookup cache | logs | 必须保留的 history/state | credentials/config 风险 |
| --- | --- | --- | --- | --- |
| Claude Code | `paste-cache`、`image-cache`；crash 后 `shell-snapshots` 有内建 sweep；`stats-cache.json` 可重建但会丢历史 totals | `debug/` | `projects/` transcripts/tool results/memory、`history.jsonl`、file history、plans/tasks | `.claude.json` 混合 OAuth/config/project trust；settings/plugins/skills 不清 |
| Grok Build | 官方未列独立 cache/download root | `.grok/logs/`，但无公开 retention | `.grok/sessions/`、memory、rewind、plans、compaction checkpoints | `auth.json`、config、plugins/skills/agents 不清 |
| Antigravity CLI | `cache/last_conversations.json` 是 workspace→conversation lookup | transcript 路径含 `logs`，但它本质是会话历史 | `brain/<conversationId>` transcripts/artifacts；task/subagent 状态 child paths 未完全公开 | OS keyring、MCP OAuth tokens、settings、MCP config、plugins/skills 不清 |
| Gemini CLI | 官方未列独立 cache/download root | Activity logs 与 session 联动，但 child paths 未公开 | `.gemini/tmp/<hash>/chats`、manual/restore checkpoints、shadow Git、tracker state | Cached credential path 未公开；settings/commands/skills/extensions 不清 |
| Codex CLI | 官方未给可安全清理的完整 cache allowlist | `log/`、`logs_2.sqlite*` 无公开 retention/并发契约 | sessions、archives、indexes、state DB；CLI/Desktop 可能共享根 | `auth.json`、config、rules/skills/plugins 不清 |

## 对 Foal discovery 的直接约束

1. 不可用顶层目录名做 eligibility：`.gemini\tmp` 明确含 resumable conversation 和
   restore checkpoints；Antigravity 的 `.system_generated\logs` 明确含 transcript。
2. credentials/config 必须作为结构性排除：Claude `.claude.json`、Grok `auth.json`、
   Antigravity keyring/MCP tokens/MCP configs、Gemini cached auth 都不能进入候选。
3. 当前官方证据最强的窄目标只有 Claude 明确命名并自带 retention 的部分 cache/log
   children，以及 Antigravity `cache\last_conversations.json` lookup。Codex 的 `.tmp`、
   plugin/cache 和诊断数据库仍是活跃或无契约状态，不能因名称或体积进入 P1。
4. Grok 官方源码证明 `downloads/` 是 mixed lifecycle：内建 updater 明确保留 current +
   previous，并保护并发临时文件；whole-root cleanup 排除。Windows 固定 staging payload
   仍有安装并发窗口，暂不批准。当前证据最强的是 Grok `bin` 下官方自清理的精确
   `.old` update residue。
5. 是否值得实现，以及是否应尊重产品自己的 cleanup command/retention，需另行做产品决策。
6. Session/history 若未来只做发现，也应默认 path-free、排除 observed bytes from
   reclaimable totals；它们是敏感用户状态，不是“可重建垃圾”。

## 已确认的 P1 产品决策

- 每种 CLI agent 使用独立、产品范围的 canonical category；不能建立统一执行的
  `cli-agents` mega-category。
- `cli-agents` 未来只可作为选择别名，展开到独立注册的 categories；它不拥有候选、
  路径或删除策略。
- 该粒度决定不批准任何当前研究目标进入 catalog。每个 agent 仍须独立证明精确
  allowlist、排除项、运行时边界、planned action 和用户影响。
- 首个深入 tracer 是 Grok Build；源码补证把方向从 whole `downloads/` 收窄到精确
  Windows updater residue。
- P1 候选 category 定为 `grok-build-update-residue`。v1 只允许 Grok Windows updater
  在 `$GROK_HOME\bin` 为 `grok.exe` / `agent.exe` 创建的 exact `.old` backup family；
  whole `downloads/`、固定 staging payload、`.rollback.bak` 与所有其他 `.old` 文件排除。
  默认选择语义仍待决定。
- Category 使用 fail-closed update-quiet gate：`grok.exe` 与 `agent.exe` 在 discovery 前后
  都必须 known idle；unknown/running 整类跳过；recognized Grok updater payload 在最近一小时
  有写入时整类跳过。一小时沿用官方 updater 的 `STALE_TMP_AGE`，且长于官方 20 分钟
  download timeout。删除前还必须 fresh re-resolve `$GROK_HOME` 并验证 exact filename、
  direct `bin` parent 与 ordinary-file type；任一失败不做部分清理。
- Planned action 为 `delete_permanently`，与 Grok 官方 updater 的直接删除语义一致；CLI
  Execute 必须有每次运行的 `--allow-permanent`，TUI confirmation 必须披露并授权永久删除。
  禁止因缺少授权而 fallback 到 Recycle Bin。
- CLI 默认 Clean 不解析 Grok root 或检查 Grok process；仅 exact
  `--opt-in grok-build-update-residue`、group alias `--opt-in cli-agents` 或
  `--opt-in all` 启用。它不属于 `dev-caches`。Clean TUI 按 catalog eager preview 规则
  测量，并在可安全测量时遵循现有 permanent-category 初始选中规则；用户可取消选择。
- Root resolution：`GROK_HOME` unset 时只用当前用户 `%USERPROFILE%\.grok`；override
  仅在 non-blank、absolute、canonicalizable 且通过危险根/Protection 检查时使用。blank、
  relative、unavailable、reparse-point、dangerous 或 protected root 整类 fail closed，
  不 fallback default/CWD。只检查 resolved root 的 direct `bin` child；不查注册表、
  不运行 Grok、不搜索其他安装位置。
- Candidate filename allowlist 仅为 lowercase exact `grok.exe.old`、`agent.exe.old`，以及
  anchored `grok.exe.old.<pid>-<seq>.old` / `agent.exe.old.<pid>-<seq>.old`；`pid` 与 `seq`
  只能含十进制数字。所有其他 `*.old`、`.rollback.bak`、directory、reparse point、
  executable/plugin file 排除，禁止 wildcard-style eligibility。
- Update-quiet witness 比 candidate allowlist 更宽：检查 direct `$GROK_HOME\downloads`
  中所有 ordinary file；任意 filename 以 `grok-` 开头且一小时内写入，整类跳过，但绝不
  将该文件变成候选。missing downloads 表示未观察到活动；directory unreadable 或相关
  timestamp unknown 时 fail closed。该 gate 用安全误跳过兼容未来版本/架构/temp 命名。

详见 [ADR 0021](../adr/0021-cli-agent-cleanup-uses-product-scoped-categories.md) 与
[ADR 0022](../adr/0022-grok-build-update-residue-is-an-exact-permanent-category.md)。

## 资料局限

- Grok Build 官方源码刚于 2026-07-15 发布；本文以当前 `main` user guide 为准，
  released binary 与周期性同步源码可能有版本差异。
- Antigravity CLI 的官方仓库只公开发布/安装信息；详细路径主要来自官方产品文档，
  该文档没有给 conversation tree 的完整文件清单。
- Gemini CLI 官方文档确认 cached credential 的存在，但没有在认证页固定其文件名。
- Codex 官方 manual 抓取在本次环境返回 HTTP 403，Docs MCP 也不可用；Codex 精确路径
  结论仅采用官方仓库、官方包 CLI help 和本机非内容元数据，并明确保留不确定性。
- 本机检查仅枚举五个产品已知用户根的目录名、类型、大小和时间戳，并运行 CLI help；
  未读取 session/prompt、凭据、配置正文、keyring 或项目文件内容。本机观察只用于发现
  待验证目标，不单独证明 cleanup eligibility。
