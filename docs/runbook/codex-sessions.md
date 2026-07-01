# NexusIM Codex Sessions

本文件记录当前 Codex 协作边界，供每轮入口快速确认。

## Agent Lab

- Thread: current Agent Lab session.
- Workspace: `E:\development\IM-agent`
- Branch: `codex/agent-lab`
- Push target: `origin/codex/agent-lab`
- Scope: Agent / RAG / memory / Python AI Worker / EvidencePack / eval gate
  research, SDD, fixture-only prototype and focused eval planning.

## Main Integration

- Thread: `019ea0b0-69fd-7be1-9ec4-4b3c6247e36d`
- Role: receive Agent Lab handoff, review, summarize and merge when appropriate.
- Main integration does not assign routine next tasks unless the user explicitly asks.

## Backend Lab

- Thread: `019f1d70-33e3-7be3-a0bd-443fc36e7b4c`
- Workspace: `E:\development\IM-backend`
- Scope: backend performance, hotgroup loadtest, Docker/runtime profile and bottleneck analysis.

## Rules

- Agent Lab does not modify hotgroup loadtest, Docker runtime profile, backend
  performance experiments, `main` branch, or shared production contracts unless
  explicitly requested.
- Agent Lab may write `docs/research/`, `docs/architecture/`, `docs/sdd/` and
  explicitly marked offline experiment docs for Agent Exploration Mode.
- Completed modules are pushed to `origin/codex/agent-lab` and handed off with
  branch, commit hash, changed files, checks, risks and next suggestions.
