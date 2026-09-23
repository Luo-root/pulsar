<div align="center">
  <h3>Pulsar —— 本地优先的 AI 总管：既能干活，也能派活。</h3>
</div>

<div align="center">
  <a href="https://go.dev/"><img alt="Go 1.27.0" src="https://img.shields.io/badge/Go-1.27.0-blue.svg" /></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-green.svg" /></a>
</div>

<br />

[English](README.md) | **中文**

Pulsar 是一个**本地优先的 AI 总管**：在你自己的机器上跑一套完整的 Agent Harness，把重复的流程固化成可调用的工作流，并以开放协议把任务派给外部 Agent。

| 面 | 角色 | 内容 |
|---|---|---|
| 对内 | **完整 Harness** | 会话（可 Fork，构成会话树）、模型与凭据、工具与 MCP、技能、带审批的长期记忆、子 Agent、定时任务、日志与观测 |
| 中间 | **工作流编排** | 重复流程固化成图——可视化画布编辑、保存为 YAML；四种触发：手动 / Agent 自调用 / 定时 / 钩子 |
| 对外 | **Agent 调度中心** | 以 **ACP v1** 契约接入外部 Agent：派任务、收结果、评效果、共享上下文与技能、汇总用量 |

有两件事要提前说清楚：

- **插件是一等扩展面。** 它必须能动态加载，而且不止「接一种能力」——可以贡献命令、工作流节点、触发器、设置页，以及 **UI**。Pulsar 不是只靠 MCP 活的。
- **一切走 API。** 核心是一套本地 Go runtime；Web 界面与（后续的）桌面壳都是它的客户端，并且复用同一份前端产物。

> **本仓库当前是设计与立项形态，尚无可用构建。** 产品与架构的事实源在 [docs/design/pulsar-design.md](docs/design/pulsar-design.md)。代码随第一个里程碑落地——v0.1：能用的单机 Harness + 本地 runtime API。

## 构建在

- [pulse](https://github.com/Luo-root/pulse) —— Agent 运行时：插件内核、厂商中立的模型层、无状态 ReAct 回合执行器、工具与技能系统、记忆、flow 编排、观测
- [pulse-web](https://github.com/Luo-root/pulse-web) —— Web 框架：路由、流式、协议升级、静态托管、span 钩子

## 许可

[MIT](LICENSE)
