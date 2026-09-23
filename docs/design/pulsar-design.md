# pulsar —— AI Harness 与 Agent 调度中心（v0 讨论稿）

> 状态：**讨论稿**（未冻结）。本文是产品与架构的事实源。
> 依赖底座：`github.com/Luo-root/pulse`（Agent 运行时，按 v0.2.5 源码核对）、`github.com/Luo-root/pulse-web`（Web 面，按 v0.2.0 核对）。
> 本地源码：pulse 在 `D:/go_Code/testObject/pulse`（HEAD `49c23c9` = v0.2.5 发版合并，tree 与模块缓存一致）、pulse-web 在 `D:/Github-Project/pulse-web`。开发期用 `go.mod` 的 `replace` 指向本地，避免每次改动都发 tag。
> 标注约定：**已有** = 底座已提供机制，pulsar 只需接线；**新建** = 底座没有，pulsar 自己做；**待 spike** = 尚未真机验证。

## 决议记录

| 日期 | 决议 | 影响面 |
|---|---|---|
| 2026-09-22 | 接入外部 Agent 走**协议契约**，不自造逐个产品的适配器矩阵；契约选 **ACP v1**（不是 A2A） | §4 |
| 2026-09-22 | Web 与 Desktop **两种形态都要**，先做 Web；核心逻辑必须在本地 runtime，前端一律走 API | §2.9 / §6 |
| 2026-09-22 | 跨 Agent 的**状态与用量汇总**作为一等功能（含覆盖率口径） | §4.7 |
| 2026-09-22 | 前端 = **独立 SPA**（构建产物由 pulse-web 托管），Desktop 壳只当「指向本地 runtime 的浏览器」，同一份产物两处复用 | §6.1 / §6.4 |
| 2026-09-22 | Desktop 壳倾向 Electron（分析见 §6.4），延后到 v0.6 实现，不阻塞 v0.1–v0.5 | §6.4 / §7 |
| 2026-09-23 | Desktop 壳**定案 Electron**（用户确认） | §6.4 / §7 |
| 2026-09-23 | 仓库 = `Luo-root/pulsar`，**public + MIT**（与 pulse / pulse-web 一致） | §6.2 |
| 2026-09-23 | 部署形态 = **本地优先 + 鉴权可开可关**：v0.1 只监听 127.0.0.1、不做鉴权，但鉴权从第一天就按「可选能力」设计（作为 Agent 管理服务必然有部署需求） | §2.9 / §6 |
| 2026-09-23 | 「**会话树 + Fork**」不是要新建的能力：底座已有（`Session.Fork` + header 的 `ParentSessionID`/`SeedLength`），pulsar 只做 API 与树状 UI | §1.1 / §2.1 |
| 2026-09-23 | 工作流的作者面 = **可视化拖拽编辑器**（保存时自动生成 YAML）；YAML 与画布是**同一份定义的两个视图**，不是两条路径 | §3.5 / §3.6 / §7 |
| 2026-09-23 | **插件系统是一等扩展面**：必须能动态加载，且不止「接一种能力」——可以改 UI、加新功能（与 MCP 明确分家） | §2.10 |
| 2026-09-23 | 共享给外部 Agent 的重点 = **pulsar 独有的上下文（用户画像等）+ 长期记忆只读工具**；`question` 这类交互工具不跨进程暴露 | §4.8 |
| 2026-09-23 | 仓库从 Pulse-TUI 旧内容**直接切换**：旧代码删除（不留 legacy 分支），`master` → `main`，补 LICENSE | §6.2 |

---

## 0. 定位

pulsar 是一个**本地优先的 AI 总管**，一句话：既能干活，也能派活。

| 面 | 角色 | 内容 |
|---|---|---|
| 对内 | 完整 Harness | 会话、模型与凭据、工具与 MCP、技能、长期记忆、子 Agent、定时任务、日志与观测 |
| 中间 | 工作流编排 | 把重复流程固化成图，四种触发：手动 / Agent 自调用 / 定时 / 钩子 |
| 对外 | Agent 调度中心 | 以 ACP v1 为契约接入外部 Agent，派任务、收结果、评效果、共享记忆与技能、汇总用量 |

三部分不是三个产品，而是**一个编排层 + 一个执行层**：工作流是编排（`kernel/flow`），任务是执行（内部子 Agent 与外部 Agent 走同一个 Runner 抽象）。见 §5。

---

## 1. 能力盘点：哪些是接的，哪些是自己做的

pulse 是**库**不是产品，它刻意把产品决策留给宿主。核对源码后，这个划分非常干净：

### 1.1 已有底座（pulsar 只负责装配与接线）

| 能力面 | pulse / pulse-web 已提供 |
|---|---|
| 插件内核 | `kernel`：Context、可逆 Effect、类型化 service、事件（Emit/Waterfall/Parallel + Local 变体）、Fiber、Loader |
| 模型接入 | `llm` 词汇表 + Registry + 显式能力矩阵；`llm/openai`（Completions / Responses）、`llm/anthropic`（Messages）适配器 |
| 单回合执行 | `loop.Agent`：无状态 ReAct、`Run` / `RunStream(onDelta)`、`Result{StoppedBy}`、决策级事件、`before_tool_call` waterfall 作 HITL 挂点 |
| 工具 | `toolset` 可逆注册面（Def.Name 全局唯一、`DisposeSource` 按源批量撤销）；`toolset/builtins` 13 个中性工具；`toolset/mcp`（Client 抽象 + Source/Plugin，stdio / 任意 transport）；`toolset/lsp` 可选 |
| 技能 | `skills`：agentskills.io 装载器（`List` / `Load` / `ReadFile`、`Catalog` 两级披露、资源翻页） |
| 会话 | `memory/session`：append-only 事件日志 + fold 投影 + 冷恢复；内存 / JSONL 两 backend；导出导入；**原生 Fork（`Session.Fork(ctx, atSeq)`）+ header 带 `ParentSessionID` / `SeedLength`——会话树是底座已有模型，不是要新造的** |
| 长期记忆 | `memory` 九包：`store`（namespace 隔离 + Supersede/Revoke 状态机 + SQLite/FTS5）、`compaction`（八步压缩事务）、`assemble`（按类预算 + hybrid 召回）、`index`（可丢可重建的向量索引）、`candidate` + `reflection`（自动提炼→待审）、`selfedit`（模型写路径，默认 opt-in） |
| 编排引擎 | `kernel/flow`（数据就绪图：三态槽位、AND 前置、Skip 分支、Aspect 超时/重试、E1 Observer）+ `kernel/flow/yaml`（E2 声明式装图，`uses` 指向注册的 Run 工厂） |
| 观测 | `observability`（Bootstrap / Record / Sink / NewTraceID，信封 + 各包折叠适配）；pulse-web 另带 `ConsoleSink`（彩色行渲染） |
| 装配 | `host`：kernel 注入 → provider → 模型声明 → 工具源 → 可选会话栈 → 观测桥；`DefaultAgent` 一把出 Agent；`ToolGate` 最小 HITL 挂载、`ScopeHook` 自订阅事件 |
| Web 面 | pulse-web：`Engine` 路由/Group/中间件、CORS 一方件、`Static`、HTML 模板、`Ctx.Flush`（SSE）、`Hijack`（协议升级/WS）、span 与 otel 钩子、优雅退出钩子、`WithErrorHandler`、测试入口 |

### 1.2 pulse 明确「不做」的清单 —— 它就是 pulsar 的待办清单

| 底座刻意不做 | 原文归属 | pulsar 要做 |
|---|---|---|
| 子 Agent | `loop`：「不做子 agent」 | 派生、预算、任务账本、结果回收（§4） |
| 重试 / failover | `loop`：「归上层编排」 | 策略层，`llm.KindOf` 分类是弹药 |
| 会话存储 | `loop`：「调用方持有 history」 | 装配 `memory/session` 并落盘 |
| 请求参数补齐 | `loop`：不焊 MaxTokens/Temperature | `before_generate` 注入默认（Anthropic MaxTokens 必填） |
| 跨运行持久状态 | `flow`：「不做持久化、断点续跑、分布式执行、熔断」 | 工作流定义存储 + 运行记录 + 调度器（§3） |
| 审批面 UI | `memory`：「包提供同步 API，不做面板」 | Pending 列表 / Approve / Reject / HITL 卡片（§2.5） |
| 自动记忆的触发时机 | `memory`：compaction 手动入口、reflection/candidate **默认关** | 何时压缩、何时反思、每 N 轮还是会话末 |
| 三个 LLM seam | `memory`：Extractor / Summarizer / EmbeddingProvider 归宿主 | 实现或选型 + 路由到哪个模型 |
| 技能短表进 system | `skills`：I2 归装配层 | `Catalog` 注入 system prompt |
| `load_skill` 宿主工具 | `skills`：I3 归装配层 | 技能激活工具 |
| 写前 diff 进 HITL | `builtins`：`PreviewFn` 已登记，**默认接线归 host 装配层** | 审批卡片接线 |
| 命令逃逸面的兜底 | `builtins`：cwd 约束 ≠ 命令访问约束 | 部署层隔离策略（容器 / sandbox），不假装工具层能兜住 |
| 定时任务 | 两仓都没有（源码零 `cron` / `scheduler` 命中） | 调度器（§2.7） |
| 外部 Agent 接入 | 两仓都没有 | ACP 契约层 + 兜底适配器（§4） |

> 判断依据：`loop` 的「有意钉死」「不做」、`flow` 的「不做」、`memory` 的「宿主装配桥接点」、`skills` 的「刻意不做」、`builtins` 的「三层边界」——这些章节逐条对上了上表。**框架把机制做满、把决策留给宿主**，pulsar 的价值就在决策与产品面。

---

## 2. 第一部分：Harness 本体

按域列功能。**已**＝接线即可，**新**＝pulsar 新建。

### 2.1 会话与对话

| 功能 | 归属 | 说明 |
|---|---|---|
| 多会话：新建 / 重命名 / 归档 / 删除 / 列表 | 已 | `memory.JSONLSessionStack` + `MemoryStore.List` |
| 流式输出 | 已 | `loop.RunStream(onDelta)` → SSE（`Ctx.Flush`）或 WS（`Hijack`） |
| 多轮历史装配 | 已 | `session.Surface()` → `loop.Run` history（`host` 已接线） |
| 长会话压缩触发 | 新 | `compaction.Pressure` 判定 + `Compact` 调用时机（默认关） |
| 中断 / 取消 | 已 | `ctx` 取消 → `StoppedBy=canceled` |
| 崩溃恢复 | 已 | `session.Open` 冷恢复（合成闭合事件写回日志） |
| 会话搜索 / 导出 / 导入 | 新（接线） | `SessionStore` 接口面已给，需要 UI 与索引 |
| **Fork 会话** | 已（接线） | 底座原生：`Session.Fork(ctx, atSeq)` 把父会话前 `atSeq` 条事件当种子建子会话；子会话 header 带 `ParentSessionID` + `SeedLength`；切在 tool 组中间显式拒绝（`ErrForkSplitToolGroup`）、越界拒绝（`ErrForkBadAt`）。**pulsar 只需做 API 与入口，不改底座** |
| **会话树（树状结构）** | 新 | 按 `ParentSessionID` 组树；任意节点可再 fork；UI 需要树视图 + 分支切换（对齐 Pi 的 `/tree`、mCode 的 fork/rewind 观感）。ACP 侧的 `session/fork` 至今仍标 unstable，所以外部 Agent 的 fork 能力只作能力位，不依赖 |
| 多模态输入 | 已 | `llm` 词汇表有 `PartImage` / `PartCustom`（MIME 开放），不支持必须 `ErrBadRequest`；**docx/xlsx/pptx 需应用层先解析** |
| HITL 审批卡片 | 新 | `loop.before_tool_call` waterfall（`host.ToolGate` 已给最小挂载） |
| 向人提问 | 已（接线） | `builtins.question` 需注入 `Asker` |
| 提示词模板 | 新 | 复用 Markdown 模板 + 斜杠命令（对齐 Pi / DSH / mCode 的既有习惯） |

### 2.2 模型与凭据

| 功能 | 归属 | 说明 |
|---|---|---|
| Provider / 模型声明 | 已 | `llm.Registry.Declare` + `openai.Register` / `anthropic.Register` |
| 能力矩阵可见 | 已 | `llm` 的显式矩阵（不支持即 `ErrBadRequest`），UI 应如实展示 |
| 凭据存储 | 新 | 环境变量 / 本地密钥文件；**密钥不进 git、不进日志** |
| 按用途路由 | 新 | 主对话模型 / 子 Agent 模型 / 摘要模型 / Embedding 模型分别绑定 |
| 请求参数默认值 | 新 | `before_generate` 注入（Anthropic `MaxTokens` 必填的装配层示范） |
| 重试 / failover | 新 | `llm.KindOf` 分类驱动；跨 provider 降级策略 |
| 用量与成本统计 | 新（接线） | `request.usage` 事件 → 观测信封 → 面板；跨 Agent 汇总见 §4.7 |

### 2.3 工具与扩展

| 功能 | 归属 | 说明 |
|---|---|---|
| 内置工具开关与子集 | 已 | `builtins.Register(scope, reg, Options{Root, WriteRoots, ForbidRead, Enabled})` |
| 工作区边界 | 已 | `Root` + 读写根分家 + symlink 落点校验 |
| MCP server 增删改查 | 新 | 配置面（命令/args/env/transport）+ `mcp.ConnectCommand` / `ConnectSDK` |
| MCP 启停与健康 | 新 | `src.Sync` / `src.Detach` / `Client.Close`，加健康检查与工具目录预览 |
| 按会话/工作区挂载工具源 | 新 | `Source = "mcp.<id>"`，`DisposeSource` 批量撤销 |
| 对外暴露 pulsar 的 MCP 服务 | 新 | 供外部 Agent 经 ACP `session/new` 注入（§4.8） |
| 审批策略 | 新 | 按工具 / 按工作区 / 按风险档（`RiskReadonly` / `RiskDangerous`）配置 |
| 写前 diff 预览 | 已（接线） | `PreviewFn` / `LookupPreview` 已登记，接线归装配层 |
| 自定义工具 | 新 | **走插件系统**（§2.10）：一个插件可以同时贡献工具、命令、工作流节点与 UI；MCP 只是其中一类来源，不是全部 |

### 2.4 技能

| 功能 | 归属 | 说明 |
|---|---|---|
| 技能仓库（多根目录） | 已 | `skills.Open(root)`；多个 loader 组合 |
| 列表 / 详情 / 资源翻页 | 已 | `List` / `Load` / `ListResources` / `ReadFile` |
| 短表注入 system | 新 | `skills.Catalog(metas)` → system prompt（I2 归装配层） |
| 技能激活工具 | 新 | `load_skill` 宿主工具（I3）；`Load` 返回的 `Directory` 是脚本相对路径的根 |
| 安装 / 更新 / 启停 | 新 | 来源（本地目录 / git / 包）；启停 = 换技能根 |
| 与外部 Agent 共享 | 新 | 见 §4.8（投影到各产品约定的技能根，或经 MCP 注入） |
| 前沿披露纪律 | 已 | 装载器只给 `name`+`description`，正文按需 `Load` |

> 注意底座已钉死的语义：Skill **不是** Tool、**不是** Source 插件，不自动注册 `ToolDef`；`allowed-tools` 底座**不解析**。pulsar 不要在装载层加特例。

### 2.5 长期记忆

| 功能 | 归属 | 说明 |
|---|---|---|
| 记忆浏览（Active / Pending / Superseded / Revoked） | 新 | `store` 提供数据与状态机，UI 归宿主 |
| 审批面（Approve / Reject） | 新 | `candidate.Pending` → `Approve`(Supersede) / `Reject`(Revoke + reason 落审计) |
| 手动增改与撤销 | 新 | 只走 Supersede / Revoke（禁物理 DELETE，Put 翻状态被 `ErrStatusTransition` 挡） |
| 作用域隔离 | 已 | namespace 前缀（父读子、兄弟不互见）；user / project / agent 三类作用域 |
| 自动提炼 | 新（接线） | `reflection.Reflect` → `candidate.Extract` → Pending；预算门 + 默认关 |
| 向量召回 | 新（可选） | `index` + `index/openai`；不接 = keyword-only，功能不缺 |
| 召回可解释 | 新 | `assemble` 的按类预算与诊断面（"这条为什么进了上下文"） |
| 模型自编辑通道 | 已（opt-in） | `selfedit` 三工具 + `before_tool_call` 审批，taint 默认 `untrusted-external` |
| 指标面 | 新（接线） | 提炼率 / 批准率 / 召回命中 / token 成本 / 污染拒绝率（率值计算归宿主） |

> 四条铁律必须原样传下去：event-sourced、model-visible means logged、压缩是事务不是删除、**记忆管理权在宿主管线 + 审批**。没有「全自动记忆」。

### 2.6 子 Agent

底座零支持（`loop` 明写不做），全部新建。但**不要为它单独造一套**——它与外部 Agent 调度是同一个抽象，见 §4.4 / §5。

| 功能 | 说明 |
|---|---|
| 派生语义 | 模型、工具子集、工作目录、步数/预算上限、是否继承记忆 |
| 任务账本 | 谁派的、干什么、什么状态、用了多少 token、产物在哪 |
| 结果回收 | 结论 + 产物 + 成本汇入父会话；原始轨迹归档 |
| 并发与配额 | 并行上限、超时、取消传播（父取消 → 子取消） |
| 与外部 Agent 统一 | 内部子 Agent 与 ACP agent 共用同一 `Runner` 接口与账本 |

### 2.7 定时与触发

| 功能 | 说明 |
|---|---|
| 调度器 | cron 表达式 + IANA 时区 + 一次性/周期任务 |
| 触发目标 | ① 一段 prompt ② 一个工作流 ③ 一个外部 Agent 任务 |
| 运行记录 | 每次触发的输入、状态、耗时、产出、失败原因 |
| 失败处理 | 重试策略、超时、通知（webhook / 飞书） |
| 持久化 | 重启后未触发的任务不丢；错过窗口的补偿策略要显式 |

### 2.8 日志与观测

| 功能 | 归属 | 说明 |
|---|---|---|
| 观测信封 | 已 | `observability.Bootstrap` + `Record` + `Sink` |
| 文件 sink + 轮转 | 新 | ConsoleSink 已有，落盘 sink 要写 |
| 轨迹回看 | 新 | 会话事件 ↔ 观测记录，用 traceID 串起来 |
| 成本 / token 面板 | 新 | 本地用量 + **跨 Agent 汇总**（§4.7） |
| 调试入口 | 已 | `Engine.Debug(path)` |

### 2.9 配置与运维

| 功能 | 说明 |
|---|---|
| 配置面 | 单一 config（模型 / 工具 / 技能根 / 记忆 / 调度 / 工作区 / 审批策略 / 外部 Agent 清单） |
| 工作区 | workspace = 根目录 + 读写策略 + 关联技能与记忆作用域 |
| **本地 runtime 形态** | 核心能力是一套本地 HTTP API（+ SSE/WS），Web 与 Desktop 都是它的客户端（§6） |
| 备份 / 迁移 | 会话、记忆库、工作流定义、配置的可迁移形态 |
| 自举 | 内置件与技能的更新 |

### 2.10 插件系统（扩展面）

**定位先钉死**：插件 ≠ MCP。MCP 只贡献**工具**这一类能力；插件贡献的是**能力面本身**——工具、命令、工作流节点、触发器、设置页，以及 **UI 组件与页面**（用户明确要求「插件甚至可以改动 UI、增加新功能」）。所以插件不是「MCP 的别名」，它是 pulsar 的一等扩展机制。

#### 2.10.1 底座已经是一台插件内核

pulse `kernel` 与 DSH 用的 Cordis 是**同构**的：声明式条目列表 `Loader.Entry{ID, Name, Disabled, Config}` + `Reconcile(entries)` 增量调和（新增→装载、移除→卸载、`Name`/`Config` 变化→重建、仅 `Disabled` 翻转→卸载/恢复；三阶段原子换代，单个条目失败不阻断其余）+ `Plugin.Inject()/Apply(c)` 依赖驱动（依赖消失自动卸载、恢复自动重装）+ 私有作用域 LIFO 回收（卸载即撤销其中一切注册）。源码注释写得直白：「**对齐 Cordis 把 entry.config 绑定进组件 apply 的做法**」。

对照 DSH（参考实现，不是照抄对象）：

| 维度 | DSH / Cordis（Node） | pulse `kernel`（Go，已有） | pulsar 要补 |
|---|---|---|---|
| 组合单元 | profile = bundles + patch 层叠 | `[]Entry` + `Reconcile` | profile 文件（层叠 / 覆盖 / 禁用）+ 改动热生效 |
| 插件形态 | ESM 模块导出 `name` / `inject` / `apply` | 注册进 `Loader` 的 `Factory`（**编译期**） | **源码级来源**：内置 / 外部进程 / WASM |
| 依赖 | `inject: [...]`；可选依赖 `ctx.get()` | `Plugin.Inject()` + `Require[T]`；不满足即 `Inactive` 挂起等待 | 现成 |
| 卸载 | effect / fiber 自动撤销 | 私有作用域 LIFO 回收 | 现成 |
| 配置 | schemastery schema + 设置命名空间 | `Entry.Config map[string]any`（**无 schema**） | JSON Schema 校验 + 自动表单 |
| UI | 客户端插件包 + **Slot 注册表** | —（前端不在 pulse 里） | SPA 的 Slot 注册表 + 动态加载 |
| 安装面 | `dsh plugin` 转发 pnpm；UI 侧只读清单 | — | 目录扫描 + 启停 + 权限展示 |

**结论：内核不用造，要造的是「插件的来源、清单、贡献点、UI 侧」。**

#### 2.10.2 三类插件来源

| 来源 | 动态加载 | 隔离 | 能做 UI | 代价 | 定位 |
|---|---|---|---|---|---|
| **内置插件**（编译期） | ✗（要重新编译） | 无（同进程同权限） | — | 零 | pulsar 自己的功能面就是这么装配的，不是给用户写的 |
| **外部进程插件** | ✓（放进目录即用） | 进程边界（**不是沙箱**） | 可带浏览器半 | 通信协议 + 进程生命周期 | **首推的用户扩展面** |
| **WASM 插件**（`wazero`，纯 Go 无 CGO） | ✓ | **真沙箱**（只能碰宿主导入面） | 同上 | 导入面设计 + 调试成本 | 「敢装别人写的插件」时的档位 |

**明确不做**：Go 原生 `plugin` 包（`.so` / `.dll`）——Windows 支持差、要求宿主与插件**完全一致的 Go 版本与构建参数**、且**卸载不安全**（类型与内存收不回）。三条都是硬伤，任何时候都不要把它当兜底方案。

外部进程插件的协议形状：一个可执行命令 + stdio 上的换行分隔 JSON-RPC（与 MCP 同族的**形态**，但**语义面更宽**——MCP 的握手只谈工具）。握手先 `initialize`（协议版本 + 贡献清单 + 权限声明），宿主按声明**动态注册**：工具进 `toolset`、命令进命令表、工作流节点进节点注册表、UI 半进 SPA 槽位。

#### 2.10.3 插件包形状（清单草案）

```
plugins/com.example.notion/
├── pulsar-plugin.yaml      # 清单（唯一必需文件）
├── schema.json             # 配置 schema（可选，驱动自动表单）
├── plugin.wasm             # kind: wasm 时必需
├── server.js               # kind: process 时的入口（任何语言皆可）
└── client.js               # 浏览器半（ESM），可选
```

```yaml
id: com.example.notion        # 全局唯一；工具名 / 命令名都带这个前缀，避开 toolset 的全局唯一约束
name: Notion 集成
version: 0.1.0
runtime:                      # 服务端半，三选一
  kind: process               # builtin | process | wasm
  command: ["node", "server.js"]
  # kind: wasm  → entry: plugin.wasm
ui:                           # 浏览器半（可选；它存在就意味着「能改 UI」）
  entry: client.js
  slots: ["settings.section", "sidebar.footer.action"]
contributes:
  tools: true                 # 经协议注册工具
  commands: ["notion.sync"]   # 命令面板与 CLI 共用
  workflowNodes: ["notion.sync"]   # 工作流节点（§3.3 白名单里的一项）
  triggers: ["webhook"]
config: ./schema.json
permissions:                  # 声明式，安装时展示；只有 wasm 会强制
  fs: ["$workspace"]
  exec: false
  net: ["api.notion.com"]
```

#### 2.10.4 UI 扩展点（Slot 注册表）

前端只有**一个**扩展机制：**槽位注册**。宿主先声明槽，插件往槽里挂组件；宿主没有的槽，插件挂不上（**未知槽显式报错**，不做字符串分发的逃生舱——与 `llm` 能力矩阵同源）。槽位协议照 DSH 的四种：`single`（独占）/ `list`（有序列表）/ `keyed`（按 key 覆盖或补充）/ `chain`（链式，注册方自荐 `select`）。

| 槽名 | 协议 | 用途 |
|---|---|---|
| `route` | list | 插件自己的页面（如 Notion 同步面板） |
| `sidebar.footer.action` | list | 侧栏底部动作 |
| `settings.section` | list | 设置页分区（插件配置） |
| `conversation.turn.tail` | list | 对话轮尾部（评分、导出、分享） |
| `tool.call.view` | keyed（工具名） | 自定义工具调用渲染器 |
| `message.renderer` | keyed（part 类型） | 自定义消息块（图表、卡片、diff） |
| `workflow.node.panel` | keyed（节点类型） | 工作流节点配置面板（§3.6 的编辑器要用） |
| `command.palette` | list | 命令面板条目 |
| `shell.overlay` | list | 全屏 / 抽屉式面板 |

**加载方式**：runtime 把启用插件的浏览器半当静态资源提供（`/plugins/<id>/client.js`，ESM）；SPA 启动拉 `GET /api/plugins` → 逐个 `import()` → 插件调用宿主 API `pulsar.ui.register(slot, {id, order, Component})`。**共享依赖由宿主统一提供**（React 等只从宿主命名空间取，插件不许自带框架副本）；清单变化后只重载受影响的插件，不刷新整页。

#### 2.10.5 配置与表单

`Entry.Config` 是 `map[string]any`，**没有类型也没有校验**。pulsar 在这一层补：JSON Schema 声明 → 保存前校验 → **自动生成基础表单**（DSH 只做模型层，明确说「不生成面板，各功能自己写表单」；我们第一版直接给 schema 驱动的表单，插件想自定义就占 `settings.section` 槽自己画）。schema 与值的读写走 `Configurable`（`kernel` 已有：实例私有的配置交接点，不经过全局服务仓库，因此插件之间不会互相覆盖、卸载互不影响）。

#### 2.10.6 信任边界（不许含糊）

- **内置插件与 WASM 之外的插件都没有沙箱**：进程插件是「宿主起的另一个进程」，它能干的事就是它能干的一切；前端插件 = **在用户浏览器里执行任意代码**（读得到页面上的全部数据、带得到会话凭据发请求）。
- 纪律是：**安装即信任 + 权限声明必须在安装时展示**（`permissions` 只做展示与策略，不做强制隔断——强制隔断只有 WASM 有）。带浏览器半的插件按「能改 UI」对待，安装走显式确认。
- **不给模型装插件的能力**：DSH 对「模型动态写的插件」用 `node:vm` 跑，它自己的文档写着「**不是安全边界**，把动态包当 bash 访问」——pulsar 直接不提供「模型写插件 / 装插件」这条通道，插件只能由人装。
- 插件注册的工具与命令**一样过审批门**（§2.3 审批策略、§4.9 跨 Agent 一致化）：插件不是审批的旁路。

#### 2.10.7 与 `toolset` / 工作流 / 调度器的接缝

| 插件贡献 | 落到哪 | 撤销方式 |
|---|---|---|
| 工具 | `toolset` 注册，`Source = "plugin.<id>"` | `DisposeSource` 批量撤销（底座已有） |
| 工作流节点 | `workflow` 节点注册表（§3.3 白名单） | 条目卸载即摘除；仍在引用它的工作流**在加载期显式拒绝** |
| 命令 | 命令表（CLI 与 Web 命令面板共用） | 条目卸载即摘除 |
| 触发器（webhook 等） | 调度器（§2.7） | 同上 |
| UI 组件 | SPA 槽位（§2.10.4） | 清单变更后重载该插件的浏览器半 |

> 与「自定义工具」（§2.3 原先那个「待定」）的关系：这一节**取代**了它。用户想写扩展时，写的是插件（能同时带工具、节点、UI），而不是「往宿主里塞一个 Go 函数」。

---

## 3. 第二部分：工作流编排

### 3.1 先把 `flow` 的边界钉死

`kernel/flow` 是**一次运行内**的数据就绪编排：节点声明 `Requires`（AND 前置）/ `Provides`，运行时一次提交全部节点，数据到达即继续；槽位三态 `pending / ready / skipped`；Skip 是正常分支结果不是失败；节点 error 记录首错并取消整图。

它**刻意不做**：持久化、断点续跑、分布式执行、熔断、跨运行调度、OR 依赖、节点重跑。

所以「重复性流程 → 可调用工作流」缺的那一层，正是 pulsar 的 `workflow` 子系统。**不要把持久化塞进 flow**——那会撞上被冻结的槽位契约。

### 3.2 工作流的三个组成

```
定义(Definition)  ──▶  运行(Run)  ──▶  记录(RunRecord)
  YAML 拓扑 + 参数 schema      flow.Graph + Seed       节点状态/输出/耗时/错误
  版本 + 触发声明              Aspect: Timeout/Retry
```

- **定义**：E2 既定形态——YAML 只描述拓扑（`id` / `uses` / `requires` / `provides`），`uses` 指向 Go 侧注册的 Run 工厂；`provides` / `requires` 的 Key 用 `{name, type}` 对账（同名异型显式报错）。外加 pulsar 自己的元数据：参数 schema、默认输入、可用触发器、版本。
- **运行**：`flowyaml.Load` → `SeedPlan.Apply`（外部输入走 `flow.Seed`）→ `Graph.Run`，节点生命周期挂 `flow.Observer`（含官方 `NewRecordObserver` 折观测信封）。
- **记录**：运行的输入、每节点状态与耗时、输出、错误、触发来源、关联会话——独立 run store。

### 3.3 节点词汇表（关键设计）

工作流能用的步骤必须是**显式注册、有类型契约**的白名单。理由：与 `llm` 的能力矩阵同源——**未知即显式拒绝，不做字符串分发的逃生舱**。节点类别：

| 类别 | 例子 | 备注 |
|---|---|---|
| 计算 | 模板渲染、JSON 变换、条件分流 | 条件分流用 `Skip`，不用 OR 调度 |
| 工具 | 把某个 tool（builtins / MCP）包成节点 | 需声明 Risk 与审批策略 |
| 内部 Agent | 调 `loop.Agent` 跑一轮/多轮 | 继承 pulsar 的模型与工具面 |
| 外部 Agent | 派任务给 ACP agent（Codex / Pi / DSH / mCode…） | 产出结构化结果（§4） |
| IO | 文件读写、HTTP、git、通知 | 写操作要吃审批策略 |
| 控制 | 并发上限 `WithMaxRunning`、`Timeout`、`Retry` | 切面已由 flow 提供 |

### 3.4 四种触发

| 触发 | 形态 | 说明 |
|---|---|---|
| 手动 | CLI `pulsar flow run <name> -i k=v` / Web 按钮 | 同步等结果或后台跑 |
| Agent 自调用 | 工作流暴露为工具（`run_workflow` 或每流程一工具） | 注意 `toolset` 语义：`Def.Name` 全局唯一，多流程要么一工具带 name 参数，要么按 Source 分组注册 |
| 定时 | cron 任务指向一个工作流 | 复用 §2.7 调度器 |
| 钩子 | HTTP webhook / 文件变化 / git hook / 观测事件 | 事件触发要防抖与幂等 |

### 3.5 作者面：画布与 YAML 是同一份定义的两个视图

**决议（2026-09-23）**：人工创建走**可视化拖拽**，保存时**自动生成 YAML**；Agent 生成 / 修改的是同一份 YAML。两者**不是两条产品路径**，而是同一份定义的两种视图——这条纪律直接决定实现方式：

| 方向 | 要求 |
|---|---|
| 画布 → YAML | 生成器必须写出**稳定**的 YAML（key 顺序、缩进、注释位置固定），否则每次保存都是一次全文件 diff，进 git 就没法 review 了 |
| YAML → 画布 | 打开时按 YAML 重新解析成图；**YAML 里有而画布表达不了的东西**要么在画布上如实显示，要么在加载时说清楚——不静默丢 |
| 谁是真源 | **YAML 是持久形态**（进 git、可 review、可被 Agent 生成与修改）；画布的坐标只作 UI 元数据单独存放，**不进语义** |

于是 §3.3 的节点白名单不只是「校验用的清单」，它同时是**编辑器的节点面板**：节点注册表要能回答「有哪些节点、属于哪个类别、输入输出 Key 是什么类型、参数 schema 长什么样」——画布、校验器、Agent 生成器**共用同一份**答案。

第三条路（后置，不在 v1）：**从会话沉淀**——把一次成功的多步会话提炼成工作流定义。信息量最大、最难，等前三件稳了再说。

### 3.6 可视化编辑器（v1 形态）

```
┌───────────────┬──────────────────────────────┬────────────────────┐
│ 节点面板       │  画布（DAG）                  │  参数面板           │
│ 按类别分组     │  连线 = requires / provides   │  schema 驱动的表单   │
│ 搜索 / 拖入    │  类型不符拒绝连线              │  插件可占槽自定义     │
├───────────────┴──────────────────────────────┴────────────────────┤
│ 校验结果（与 flowyaml 同一套规则）                                  │
└───────────────────────────────────────────────────────────────────┘
```

- **连线即类型对账**：拖拽时的连线校验 = `flowyaml` 声明期校验的**前移**（同名异型、重复 Provide、来源冲突、环）。**一道规则、两处执行，不许两套判据**——画布放行的图，加载期也必须通过。
- **参数面板**：默认 JSON Schema 表单；插件节点可在 `workflow.node.panel` 槽替换成自己的面板（§2.10.4）。
- **预览**：只做「解析后的图 + 节点会用到的 Key / 类型」，**不做单步执行与断点**。
- **调试**：跑一次 → 按 `flow.Observer` 的三态给节点上色（状态 + 耗时 + 错误）。运行视图与编辑视图分离，不在画布上直接断点。

### 3.7 明确不做（第一版）

断点单步、跨运行的状态复用、分布式执行、子图嵌套、多人协作编辑。理由：没有真实需求前不引入；`flow` 本身也不做持久化与节点重跑。

---

## 4. 第三部分：Agent 调度中心

这一部分要先分清两件事——「管理所有 Agent」实际混了两个不同的动作：

| 动作 | 含义 | 风险 | 建议 |
|---|---|---|---|
| **派活（dispatch）** | 把任务交给外部 Agent 执行，收结果 | 低（隔离在任务边界内） | 第一版主力 |
| **托管（supervise）** | 接管外部 Agent 自己的配置 / 技能 / MCP / 会话 | 高（改别人配置 = 破坏性） | 第一版**只读**（探测与展示）+ 只投影约定的共享面 |

### 4.1 接入契约：以 ACP v1 为准，不自造适配器矩阵

**决议**：pulsar 不逐个产品写私有适配器，而是**实现一份协议契约**——谁满足契约，谁就能被接入、被派活。

选 **ACP（Agent Client Protocol，Zed + JetBrains 发起，Apache-2.0，wire v1）**。横向对比（事实已核对）：

| 判据 | **ACP** | A2A | MCP | AGENTS.md |
|---|---|---|---|---|
| 定位 | 客户端 ↔ Agent | Agent ↔ Agent | Agent ↔ 工具 | 仓库 ↔ Agent（文件约定） |
| 传输 | JSON-RPC 2.0 over stdio（NDJSON）；远程 HTTP/WebSocket 仍在演进 | HTTP + SSE + gRPC + Agent Card | JSON-RPC over stdio / HTTP | 纯 Markdown |
| 版本 | **wire v1 稳定**（SDK crate 独立迭代；协议版本由 `initialize` 交换，老客户端仍能与新 agent 通话） | v1.0.1 | 日期化修订 | 无版本 |
| 目标产品支持度 | **30+ agent 已认**：Claude Code、Codex、Gemini CLI、**Kimi CLI**、OpenCode、Cline、Goose、Cursor、Copilot、Junie、Kiro… | 本类产品**零原生说话者**，被评「企业惯例，coding-agent 世界可以继续忽略」 | 近通用 | 60k+ 仓库；Codex / Pi / DSH 都读 |
| 结论 | **选它** | 暂不追 | 用作工具面（§2.3） | 用作共享面（§4.8） |

**pulsar 在 ACP 里扮演 client**（即 Zed 的那个位置）：

```
pulsar ──spawn──▶ agent 子进程
   │  initialize            ← 协商协议版本 + 能力
   │  session/new           → cwd + mcpServers
   │  session/prompt        → 用户任务
   │  ◀── session/update    流：agent_message_chunk / agent_thought_chunk /
   │                            tool_call(_update) / plan / usage_update /
   │                            available_commands_update / current_mode_update
   │  ◀── session/request_permission   审批请求（Agent → Client）
   │  session/close
```

已有先例证明这条路可走：社区存在用同样方式做**跨项目并行 ACP 编排**的项目。

**一个巧合级别的契合**：ACP 的能力协商（`initialize` 返回能力集，presence = 支持、absence = 不可用，且规范要求客户端**必须先检查能力再调用可选方法**）与 pulse `llm` 的显式能力矩阵是同一种设计价值观——**不支持就明说，不静默降级**。所以 Runner 的 `Capabilities` 不该由 pulsar 手工枚举各产品差异，而应**从 `initialize` 的结果派生**。

### 4.2 各目标产品的接入路径（已核对来源）

| 产品 | ACP 路径 | 兜底路径 | 已核对的事实 |
|---|---|---|---|
| **MiniMax Code** | **原生** `mcode acp`：ACP v1 over stdin/stdout NDJSON，无需 HTTP 服务；支持 create / load / resume / close Session | `mcode exec`（headless：`--output-format json\|stream-json`、`--output-schema`、`--output-last-message`、`--effort`、`--permission`、`--timeout`、`--max-steps`）；另有 `mcode exec review` 专做改动审查 | 官方文档与 npm 包说明一致；已有第三方（LobeHub）按这条路接了实验性 provider |
| **DeepSeek Harness** | **原生** `@deepseek-ai/dsh-acp`（ACP v1 over stdio；`dsh --profile acp`）。新版还带 `session/list` / `resume` / `close` / `set_config_option`，并在 `session/update` 里发**上下文用量** | `dsh --profile headless "<task>"`（退出码 0/1） | DSH 仓库自带的 ACP 客户端就是 `dsh-subagent-acp`——**它自己就用 ACP 派子 Agent**；老版桥明确不上报 usage，新版才发 |
| **Codex CLI** | 社区桥 `codex-acp`（把 Codex runtime 桥成 ACP server）；Codex 已在 ACP Agent Registry 中 | `codex exec --json`（JSONL：`thread.started` / `turn.started` / `turn.completed`(usage) / `turn.failed` / `item.*`（agent_message / reasoning / command_execution / file_change / mcp_tool_call / web_search / todo_list）/ `error`）、`--output-schema`、`-o`、`resume [--last\|<id>]`、三档 sandbox、`--ask-for-approval` | 另有 `codex mcp-server` 把 Codex 暴露成 MCP（`codex` / `codex-reply`） |
| **Pi** | 社区适配器 `pi-acp`（spawn `pi --mode rpc` 再桥到 ACP；周下载 8 万+，作者自称 MVP）；已在 ACP Registry，支持 Terminal Auth | Pi 自己的 `pi --mode rpc`（JSONL over stdin/stdout）与 Node SDK | **已知缺口**：pi-acp 不做 ACP 的文件系统/终端委托、**不做 `session/request_permission` 审批门**（pi 本地执行工具，不在 RPC 上暴露执行前意图）；ACP 会话可以回 pi 里 `/resume` |

> 一个重要的共性：**ACP 的 `session/new` 带 `mcpServers` 参数**——所以"把 pulsar 的记忆/技能服务注入给外部 Agent"在协议层就是一次 `session/new`，不必逐个产品改配置。pi-acp 已经把 ACP 传来的 MCP server 翻译成 pi 的会话级临时配置，并明确**不写** `/.pi/mcp.json`（避免污染用户自己的配置）——这条纪律值得学。

### 4.3 兜底适配器：协议覆盖不到的地方

协议优先 ≠ 只做协议。三档接入，按可观测性从高到低：

1. **ACP 优先**：有 ACP 面就走 ACP，按 `initialize` 协商结果降级。
2. **自有结构化接口**：没有 ACP 但有 headless JSON / 事件流（`codex exec --json`、`pi --mode rpc`、`mcode exec --output-format stream-json`）→ 解析成同一套归一化事件。
3. **裸进程兜底**：只有一次性命令 → 退出码 + stdout + 工作目录 diff（能力矩阵里如实标注「低可观测」）。

三档对上层暴露的都是同一个 `Runner`，能力差异只体现在 `Capabilities` 里。

### 4.4 统一抽象：Runner

内部子 Agent 与外部 Agent 是同一个接口的两种实现。形状对齐 pulse 的适配器惯例（`llm` 的 Factory/Register、`toolset/mcp` 的 Client+Source）：

```go
type Runner interface {
    Describe() Capabilities   // 由 ACP initialize 派生，或由适配器显式声明
    Start(ctx, Task) (Run, error)
}

type Run interface {
    ID() string
    Events() <-chan Event          // 归一化事件流
    Wait(ctx) (Result, error)
    Cancel(ctx) error
    Answer(ctx, PermissionID, Decision) error // ACP request_permission / 兜底策略
    Resume(ctx, SessionRef) (Run, error)
}

type Task struct {
    Prompt       string
    Workspace    string
    Model        string           // 可选；ACP session/set_config_option 或适配器参数
    Policy       Policy           // 审批档 / 沙箱档 / 超时 / 最大步数
    OutputSchema []byte           // 可选：要求结构化终态
    Share        ShareSpec        // 经 session/new 注入的 MCP servers / 投影的技能
}
```

**能力矩阵显式、不支持就显式报错**——与 `llm` 的契约同源：`Resume` 不支持返回哨兵错误而不是静默重跑；`OutputSchema` 不支持就不要假装约束了输出；`request_permission` 不可用（如 Pi）时必须在 `Capabilities` 里标出来，让上层决定是否放行。

### 4.5 派活与回收

1. **任务书生成**（可选）：用内部 Agent 把模糊需求写成任务书（目标、约束、验收标准、禁改范围）。这是 pulsar 相对"直接敲命令行"的增量价值。
2. **派发**：写入任务账本（谁/什么/何时/工作目录/预算/策略），`Runner.Start` 执行。
3. **进度**：归一化事件流 → 观测信封，**同一条 traceID 串起来**（与 pulsar 自己的会话、工作流运行同链）。
4. **回收**：终态 + 产物 + 原始事件归档。原始轨迹**必须留**——它是评估与审计的唯一证据。
5. **隔离**：工作目录、凭据注入范围（只给这个任务需要的）、审批档位。

### 4.6 完成效果评估

分三层，逐层可落地，不要一次做到"AI 打分"：

| 层 | 判据 | 客观性 |
|---|---|---|
| 机械层（必做） | 退出码、是否超时/取消、产物是否存在、改了哪些文件、diff 规模、跑了哪些命令、token 与成本 | 全客观 |
| 验证层（按任务配） | 指定的验收命令（测试 / lint / build / 自定义脚本）在**任务结束后**真跑一遍，结果纳入 Result；`mcode exec review` 这类产品自带审查也可直接复用 | 全客观 |
| 评审层（可选） | 内部 Agent 读 run 记录 + diff + 验收结果 → 出评估与理由，产物入库 | 主观，需标注是判断不是事实 |

关键设计：把「一类任务的验收标准」沉淀成**任务模板**（task template = 任务书骨架 + 验收命令 + 评估 rubric）。这样评估不是每次临时判断，而是可复用资产；同类任务多次派发后可以横向比较。

### 4.7 用量与状态汇总（跨 Agent 的总账）

要「每个 Agent 的状态统计 + token 用量，汇总出所有 Agent 的总用量」，难点不在聚合，而在**某些 Agent 根本不上报**——若按 0 计入，总账就是假的。

三条采集路径：

| 来源 | 内容 | 可靠性 |
|---|---|---|
| ACP `session/update` → `usage_update` | `used`（当前上下文 token）、`size`（上下文窗口）、`cost{amount, currency}`（会话累计成本）；另有 per-turn `Usage`（input / output / cacheRead / cacheWrite / thought / total） | 规范里**标 UNSTABLE**，且实现方可选发——DSH 老版桥明确不上报，Pi 适配器是"每个回合结束后上报" |
| 适配器事件流 | Codex `turn.completed.usage`（input / cached_input / output）；各产品 headless JSON 里的用量字段 | 产品私有字段，逐版本可能变 |
| pulsar 自己 | 内部 Agent 走 `llm` 的 usage 事件 | 100% 可得 |

设计结论：

- **账本记「口径」而不只记「数字」**：每条用量记录带 `source`（`acp.usage_update` / `adapter.codex.turn` / `internal.llm`）与 `coverage`（完整 / 部分 / 估算——`assemble` 的 nil `TokenCounter` 就是估算）。汇总面板显示**总量 + 覆盖率**，而不是一个看起来完整的总数。
- **成本必须标币种**：ACP 的 cost 带 ISO 4217 `currency`，各产品计价单位与缓存折扣口径不同，不能直接相加——**同币种内相加、跨币种分列**。
- **状态统计**从同一套归一化事件派生：任务队列长度、运行中 / 待审批 / 已完成 / 失败、每 agent 的近期成功率与平均耗时、当前会话数与上下文占用。
- 三条路径最终汇入**同一张用量/状态账本**，与工作流运行、内部 Agent 共用（§5）。

### 4.8 记忆与技能的共享

**先钉死共享什么（2026-09-23 决议）**：

1. **pulsar 独有的上下文**——用户画像、偏好、项目约定。这是**外部 Agent 不可能自己拥有**的东西，也是「把活派出去却不掉上下文」的关键。派活时先给这一份，而不是把整个记忆库倒给对方。
2. **长期记忆的只读工具**——让外部 Agent 在干活途中按需检索「以前这类问题怎么处理的」；干完活遇到的问题与结论**回流**到 pulsar（写回走归档 → 反思 → 审批，§2.5 的四条铁律不变）。
3. **不共享**：`question`（AskUser）这类**交互工具不跨进程暴露**——它要宿主 UI 与人同时在场的语义，外部 Agent 拿到它只会卡住；宿主自己的会话、审批、凭据面同样不暴露。

三层机制，从零协议到协议化：

**① 文件约定层（第一版必做，全产品通吃）**

| 共享物 | 载体 | 覆盖面 |
|---|---|---|
| **用户画像与偏好** | pulsar 投影出的画像片段（`AGENTS.md` 段落或独立文件） | 走 `AGENTS.md` 的产品都读得到；这是「独有上下文」最省事的落点 |
| 用户习惯与项目约束 | `AGENTS.md` | Codex、Pi、DSH、mCode 等普遍读取（Claude Code 是已知例外） |
| 技能 | Agent Skills 目录（agentskills.io：子目录 + `SKILL.md`） | Pi、DSH、MiniMax Code 原生支持；与 pulse `skills` 包同一规范 |

做法：pulsar 侧维护 **canonical 源**（记忆库投影出的约束 + 技能中心的技能），再**投影**到各产品约定的落点。需要一张「产品 × 落点路径 × 格式」的投影矩阵——这是**待 spike** 的实打实工作：每个产品都要对官方文档 + 真机验证（哪一层目录、优先级如何、能否软链、要不要重启生效）。

**② MCP 层（协议原生注入）**：pulsar 起一个本地 MCP server，暴露**只读**的记忆检索（即上面第 2 条）与技能读取；派活时通过 ACP `session/new` 的 `mcpServers` 参数注入——**一次协议调用，全 ACP 生态通用**。写一律走审批，第一版不给写。

**③ 协议层（更后）**：A2A（等它有原生说话者）、ACP 远程形态（仍在演进）。

**反向（外部 Agent → pulsar）**：把外部 Agent 的会话与轨迹**归档进 pulsar**，而不是让它直写记忆库。这样"别的 Agent 干过的活"成为可检索、可反思的资产：归档 → `reflection` → `candidate` → 审批 → 进 Active。这正好落在 `memory` 的四条铁律内（没有来源不进 active memory、未过审批不进上下文）。

### 4.9 安全边界（不能省）

- 外部 Agent 会真的改文件、跑命令、联网 → 审批档位 + 工作目录隔离 + 兜底（DSH 官方明确：⚠️ 沙箱只控文件系统，网络与进程不在内）。
- 审批要**跨 Agent 一致化**：ACP 的 `session/request_permission` 是标准闸，但**不是所有适配器都实现**（Pi 明确没有）。所以"这个 agent 能不能被审批门拦住"必须是能力矩阵里的一等字段，不能默认它有。
- MCP 注入即信任边界：ACP 规范里 client 传的 MCP server 属于**可信进程配置**（stdio 命令/环境变量能起进程，HTTP header 能带凭据）——pulsar 注入自己的服务时要走最小权限。
- 凭据最小化：不要把用户所有密钥同时暴露给所有 Agent。
- 全量留痕：派发、策略、结果、原始事件。

---

## 5. 统一抽象：工作流（编排）与任务（执行）

```
工作流(flow.Graph)                    ← 编排：数据就绪、AND、Skip 分支、超时重试
   │  节点类型之一：执行任务
   ▼
任务(Task) ──▶ Runner ──┬── runner/local   = 内部子 Agent（pulse loop.Agent + 工具子集 + 记忆）
                        ├── runner/acp     = 任何 ACP v1 agent（mCode / DSH / Codex 桥 / Pi 桥 / …）
                        └── runner/exec    = 兜底：一次性 headless 进程（无 ACP 面时）
   ▲
   │  触发面：手动 / Agent 工具 / cron / 钩子
```

这套统一的直接收益：

- 任务账本、并发控制、预算、结果回收、评估、用量汇总、留痕 —— **只做一套**，内部与外部共享。
- **新增一个产品支持不再是写一个适配器**：如果它讲 ACP，`runner/acp` 直接就能用；只有不讲协议的产品才需要兜底适配器。这是「协议优先」最实际的回报。
- 工作流的「执行任务」节点天然同时支持内部与外部 Agent——这是第 2 部分与第 3 部分的接缝，也让"定时派活给 Codex/mCode"变成一句话的事。

---

## 6. 架构与形态

### 6.1 形态决策：一个本地 runtime，两个客户端

Web 与 Desktop 都要，**先做 Web**。这条要求决定了架构的硬约束：

```
                    ┌──────────────── pulsar runtime（Go，本地进程）────────────────┐
                    │  harness（会话/模型/工具/技能/记忆/审批/调度/日志）              │
   Web UI ──────────┤  workflow（flow + flowyaml + 触发器 + 运行记录）                 │
   (pulse-web)      │  orchestrator（Runner + ACP client + 任务账本 + 用量账本 + 评估） │
      HTTP/SSE/WS   │  ← 全部能力只经 HTTP API 暴露                                  │
   Desktop 壳 ──────┤                                                                │
   (同一套前端资源)  └────────────────────────────────────────────────────────────────┘
```

- **API 优先是纪律**：任何能力先有 API，前端不许绕过 API 直接摸内部对象——否则 Desktop 要重做一遍。
- Web：pulse-web 承载 API 与页面（`Engine.Static` 托管资源；流式走 `Ctx.Flush`；WS / 协议升级走 `Hijack`；观测走 span 钩子与 `ConsoleSink`）。
- Desktop：同一套前端资源 + 一个壳（**已定 Electron**，见 §6.4），负责启动或连接本地 runtime。

### 6.2 仓库布局

**仓库切换（2026-09-23）**：`Luo-root/pulsar` 建于 2026-05-20，里面是上一代 **Pulse-TUI**（基于 pulse v1 的 Bubble Tea 终端界面：多厂商模型 / safe-auto 工具审批 / MCP / SQLite 记忆 + 向量检索 / skills / `/plan`）。它是**另一个产品**，与本文的设计没有继承关系——按用户决议**直接删除**（不留 `legacy/pulse-tui` 分支；git 历史里仍可查），默认分支 `master` → `main`，补 MIT LICENSE。pulsar 的 CLI 从头写。

```
pulsar/
├── cmd/pulsar/             CLI：run / serve / flow / mem / skill / mcp / task / cron
├── internal/config/        配置与凭据
├── internal/app/           装配：kernel + host + 各能力插件（pulse 的 Use 面）
├── harness/                产品逻辑：会话 / 模型 / 工具 / 技能 / 记忆 / 审批 / 日志
├── workflow/               定义注册表 + 参数契约 + 触发 + 运行记录
├── orchestrator/           Runner + ACP client + 任务账本 + 用量账本 + 评估
│   └── runner/{local,acp,exec}/
├── web/                    pulse-web：API + 静态资源；Desktop 复用同一套资源
└── docs/design/            事实源
```

依赖纪律（对齐 pulse 自己的分层）：`orchestrator` **只依赖自己的 Runner 接口**，不 import `harness`；共享内容（记忆、技能）通过装配层注入的 seam 传递。`harness` 不感知具体是哪个外部产品。

### 6.3 本地开发

`go.mod` 用 `replace` 指向本地克隆，改动即时生效：

```
replace github.com/Luo-root/pulse     => D:/go_Code/testObject/pulse
replace github.com/Luo-root/pulse-web => D:/Github-Project/pulse-web
```

（是否把 `replace` 提交进仓库需要决策——提交会让别人 clone 后无法构建，通常做法是本地保留、或用独立的 dev 构建脚本注入。）

### 6.4 Desktop 壳选型：Electron vs Tauri

**决策前提**：壳只干三件事——启动/守护本地 runtime、开一个窗口指向 `http://127.0.0.1:<port>`、提供托盘 / 自启 / 更新。**业务逻辑全在 Go 侧**，所以这是「薄壳」选型：比的是平台集成的摩擦与运维成熟度，不是运行时性能。

| 维度 | Electron | Tauri 2 |
|---|---|---|
| 渲染引擎 | 自带 Chromium，各平台一致 | 系统 WebView：Windows = WebView2（Chromium）、macOS = WKWebView（Safari）、Linux = WebKitGTK（版本碎片） |
| 安装包 | 约 100–300 MB（Chromium + Node 随包） | 约 3–10 MB（最小可到 600 KB 级） |
| 空闲内存 | 约 150–400 MB | 约 40–80 MB |
| 冷启动 | 约 1–4 s | 约 200–500 ms |
| 壳内语言 | Node.js / TypeScript（前端同学零门槛） | Rust（壳里每加一个平台能力都要写 Rust + 权限清单） |
| 安全默认 | 需显式配 contextIsolation / sandbox / CSP | 能力制，默认拒绝 |
| 生态先例 | VS Code、Cursor、Slack、Discord、Notion、Obsidian、Figma Desktop、**MiniMax Code Desktop** | Sourcegraph Cody、Spacedrive、AppFlowy、Aptakube、1Password（部分迁移） |

> 数字是多篇 2026 对比文的共同量级（约 10× 体积、3–5× 内存），**只有量级可信**，具体百分比随应用差异很大；真要写进文档得按自己的壳实测。

**决策：Electron**（2026-09-23 用户确认）。理由按权重排：

1. **壳里的活是「平台集成」而不是「性能」**：托盘常驻、开机自启、单实例、自动更新、系统通知、文件拖拽、深链——这些在 Electron 是成熟 API 与现成库，在 Tauri 是 Rust 侧代码 + 权限声明。pulsar 是个要长期加功能的管家（定时任务提醒、后台常驻跑工作流），这一步会反复走。
2. **Rust 摩擦的性价比不划算**：壳越薄，Tauri 省下的体积与内存越不值钱（目标用户是已经装了 Go、Node 与多个 agent CLI 的开发者），而每加一个平台能力都要过 Rust 这道门。
3. **代码型 UI 对渲染一致性更敏感**：diff、等宽对齐、CJK 宽度、终端输出——WKWebView 与 WebKitGTK 的差异是真实的测试面。
4. **先例可搜**：同类产品（MiniMax Code Desktop）与主流代码工具都在 Electron 上，踩坑有答案。

**什么时候翻成 Tauri**：① 只做 Windows（WebView2 本身就是 Chromium，一致性风险归零）且把「安装包小、启动快」当卖点；② 要做移动端同构；③ 团队有 Rust 意愿。三条至少满足一条再翻。

**但这不阻塞**：v0.1–v0.5 都是「浏览器打开本地 runtime」（与 `dsh web` 同形），壳到 v0.6 才需要。真正要现在定死的是 **SPA 的取数接口形状**（API + SSE）——那才是 Desktop 复用成本的决定因素。

---

## 7. 分期（小步迭代，先清小账再攻纵深）

| 版本 | 目标 | 内容 |
|---|---|---|
| v0.1 | **能用的单机 Harness + API** | 会话（含 Fork / 会话树 API）+ 模型 + 内置工具 + 技能装载 + MCP + 日志落盘 + 记忆（session/store + 手动审批面）+ CLI + 本地 runtime API。装配形态**本来**就是插件内核（内置插件表 + profile 层叠 + 配置热生效），但这一版不对外开插件来源 |
| v0.2 | **Web 端做好** | pulse-web 接入：聊天流式、会话管理（含树视图与分支切换）、记忆审批面板、技能与 MCP 管理台、观测与用量面板、工作区管理；**SPA 内建 Slot 注册表骨架**（前端先把缝留好，代价很小，后补要返工） |
| v0.3 | **工作流 v1** | 定义（YAML = 真源）+ 节点注册表 + **可视化拖拽编辑器**（§3.6）+ 运行记录 + 手动与 Agent 工具触发 + 定时触发 |
| v0.4 | **ACP 接入 v1** | ACP client（initialize / session / prompt / update / close）+ 能力协商派生 + 任务账本 + 机械层与验证层评估 + 用量账本（含覆盖率）。**先在本机已有 ACP 面的两个产品上 spike：`mcode acp` 与 `dsh-acp`** |
| v0.5 | **插件系统 v1（动态加载）** | 清单 + 目录扫描 + 外部进程插件（协议握手、工具 / 命令 / 工作流节点 / 触发器贡献）+ UI 槽位挂载 + schema 表单 + 启停与权限展示（§2.10）。放在这里是因为它需要宿主面先长齐（工具、命令、设置页、UI 槽都有东西可挂），而工作流的节点面板正好第一个用上它 |
| v0.6 | **共享与自动化** | AGENTS.md 与技能目录投影（含用户画像）+ 经 `session/new` 注入 pulsar 的只读 MCP server + 钩子触发 + 评审层评估 |
| v0.7 | **Desktop 壳（Electron，已定）** | 复用 Web 前端资源；本机 runtime 生命周期管理（托盘 / 自启 / 更新，见 §6.4） |
| 之后 | 纵深 | WASM 插件档位、兜底 `runner/exec` 适配器、从会话沉淀工作流、pulsar 自身作为 ACP server（让 Zed/JetBrains 驱动 pulsar）、A2A 观望 |

每个版本的验收标准应是**一个能跑通的用户场景**，而不是功能面堆砌。

---

## 8. 决策台账

### 8.1 已定

| # | 决策 | 日期 | 落点 |
|---|---|---|---|
| 1 | 接入外部 Agent 走**协议契约 ACP v1**，不自造逐产品适配器矩阵 | 2026-09-22 | §4.1 |
| 2 | Web + Desktop 都要，**先 Web**；前端 = **独立 SPA**；核心逻辑在本地 runtime，前端一律走 API | 2026-09-22 | §6.1 |
| 3 | 跨 Agent 的**状态与用量汇总**是一等功能（记口径与覆盖率，不只记数字） | 2026-09-22 | §4.7 |
| 4 | Desktop 壳 = **Electron**（v0.7 实现，不阻塞 v0.1–v0.6） | 2026-09-23 | §6.4 |
| 5 | 仓库 = `Luo-root/pulsar`，**public + MIT**；旧 Pulse-TUI 内容删除、`master` → `main` | 2026-09-23 | §6.2 |
| 6 | 部署 = **本地优先 + 鉴权可开可关**（v0.1 只监听 `127.0.0.1`、不做鉴权，但按「可选能力」设计） | 2026-09-23 | §2.9 / §6 |
| 7 | **会话树 / Fork** 是底座原生能力，pulsar 只做 API 与树状 UI | 2026-09-23 | §2.1 |
| 8 | 工作流的作者面 = **可视化拖拽**，保存自动生成 YAML（YAML 是持久真源，两者同一定义的两个视图） | 2026-09-23 | §3.5 / §3.6 |
| 9 | **插件系统是一等扩展面**：动态加载、可改 UI、可加新功能；与 MCP 明确分家 | 2026-09-23 | §2.10 |
| 10 | 记忆共享第一版**只读**；共享重点 = **用户画像等 pulsar 独有上下文 + 长期记忆只读工具**；`question` 这类交互工具不跨进程 | 2026-09-23 | §4.8 |
| 11 | 「自定义工具」= **走插件**，不做「往宿主塞一个 Go 函数」那条路 | 2026-09-23 | §2.10.7 |

### 8.2 仍待拍板

| # | 问题 | 影响 | 我的建议 |
|---|---|---|---|
| A | **配置托管边界**：对外部 Agent 自己的配置（模型 / MCP / 技能）能碰多深 | 「托管」这个词最终能做到哪一步 | **只读探测 + 只投影共享面，不接管外部 Agent 的主配置**（改别人的配置是破坏性操作，且各家格式都在变） |
| B | **SPA 的技术栈**（框架 + 构建 + 语言） | v0.2 的起手式；也决定插件浏览器半怎么加载（ESM + 槽位挂载） | **React + Vite + TypeScript**：生态最大，插件挂载与共享依赖（宿主统一提供 React）最省事 |
| C | **插件来源 v1 收哪几档** | v0.5 的工作量 | v1 只做**外部进程插件 + UI 槽位**（覆盖绝大多数扩展诉求）；WASM 档位等真出现「要装别人写的插件」再上 |
| D | **v0.1 的验收场景**（首张 Issue 的完成定义） | 立项票怎么写 | 见 §8.3 候选场景 |

### 8.3 v0.1 候选验收场景

> `pulsar serve` 起本地 runtime 后，用一个真实场景走通全链：新建会话 → 绑一个真实模型 → 让内置工具在指定工作区里读写文件 → 装载一个技能并激活 → 连一个 MCP server 并用它的工具 → 全程事件按 `observability` 落盘、可用 traceID 回看 → 会话 Fork 出一个分支并各自继续 → **杀掉进程重启，会话、记忆、审批状态都还在**。

判据是**一条能跑通的场景**，不是功能面清单；上一代 Pulse-TUI 走过的取舍（safe-auto 审批档、`/plan` 规划执行）在本文的功能面里都各自有位置，可作参考但**不继承代码**。

