<div align="center">
  <h3>Pulsar — a local-first AI harness and agent orchestrator.</h3>
</div>

<div align="center">
  <a href="https://go.dev/"><img alt="Go 1.27.0" src="https://img.shields.io/badge/Go-1.27.0-blue.svg" /></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-green.svg" /></a>
</div>

<br />

**English** | [中文](README_zh.md)

Pulsar is a local-first control plane for AI work: it runs a full agent harness on your own machine, turns repeated procedures into callable workflows, and dispatches tasks to external agents over an open protocol.

| Face | Role | What it covers |
|---|---|---|
| Inward | **Full harness** | sessions (forkable, forming a session tree), models & credentials, tools & MCP, skills, long-term memory with approval, sub-agents, scheduled tasks, logs & observability |
| Middle | **Workflow orchestration** | repeated procedures become a graph — built on a visual canvas that saves to YAML; four triggers: manual / agent-invoked / cron / hooks |
| Outward | **Agent dispatch center** | external agents over **ACP v1** — dispatch, collect results, evaluate, share context and skills, aggregate usage |

Two properties worth stating up front:

- **Plugins are a first-class extension surface.** They load dynamically, and they contribute more than tools: commands, workflow nodes, triggers, settings pages and **UI**. Pulsar does not run on MCP alone.
- **Everything goes through the API.** The core is a local Go runtime; the web UI and (later) the desktop shell are both clients of it, and they reuse the same frontend build.

> **This repository currently holds the design, not a build.** The source of truth for product and architecture is [docs/design/pulsar-design.md](docs/design/pulsar-design.md) (Chinese). Code lands with the first milestone — v0.1: a working single-machine harness plus the local runtime API.

## Built on

- [pulse](https://github.com/Luo-root/pulse) — the agent runtime: plugin kernel, provider-neutral model layer, stateless ReAct turn executor, tool & skill system, memory, flow orchestration, observability
- [pulse-web](https://github.com/Luo-root/pulse-web) — the web framework: routing, streaming, protocol upgrade, static hosting, span hooks

## License

[MIT](LICENSE)
