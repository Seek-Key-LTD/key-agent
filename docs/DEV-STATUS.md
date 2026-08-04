# PicoOracle Agent — 开发状态

> 维护者：manager (nuc)
> 最后更新：2026-08-04

## 仓库

- **代码**：`~/Projects/github/adk-go/`（fork of google/adk-go）
- **Remote**：`github.com/Seek-Key-LTD/adk-go.git`（走 ghpx 代理）
- **分支**：main

## 已实现

| 模块 | 路径 | 状态 |
|------|------|------|
| Oracle memory（三层+加密+向量） | `memory/oracle/oracle_memory.go` | ✅ 可用 |
| Oracle session CRUD | `session/oracle/oracle_session.go` | ✅ 可用 |
| DDL | `memory/oracle/ddl.sql` | ✅ 已在 lake5 执行 |
| oracle-agent 示例 | `examples/oracle-agent/main.go` | ✅ smoke 通过 |
| CI flow（5 架构编译） | `.github/workflows/build-oracle-agent.yml` | ✅ 全绿 |
| UAT 方案 | `docs/uat/UAT-PicoOracle-Agent-v0.1.md` | ✅ UAT-01~04 通过 |

## 二进制

GH Actions 编译后分发到两处：

| 位置 | 地址 | 用途 |
|------|------|------|
| CF Worker（教育网入口） | `https://cernet-s3.git4ta.fun/picooracle/<commit>/oracle-agent-linux-amd64` | 公网 |
| OCA S3（教育网内网） | `oca/21579-lhhq-164014/picooracle/oracle-agent-linux-amd64` | mc cp 内网直连 |

**nuc 本机已就位**：
```
/tmp/oracle-agent-from-cernet    ← 从 CF Worker 拉的
/tmp/oa-oca-pull                 ← 从 OCA 内网拉的
~/Projects/github/adk-go/dist/oracle-agent-linux-amd64  ← 本地编译的
```

## Oracle ADB lake5

- DSN（go-ora 格式）：Consul KV `picooraclaw/dsn`
- Wallet：`/opt/picooraclaw/wallet/`
- 已建表：`PICO_MEMORY_DEEP`、`PICO_MEMORY_SHARED`、`PICO_SESSION`
- 驱动：`github.com/sijms/go-ora/v2`（纯 Go，无 CGO）

## Secrets

- Infisical project：`secret-management`（ID: `349cc5f0-e13b-446e-a940-18a7c671146a`）
- 已写入：`OCA_ACCESS_KEY`、`OCA_SECRET_KEY`
- 自动同步到 GitHub Actions secrets

## 下一步

按架构设计文档 v1.2（dumate.cn/artifacts/4hrtktacugj6）：
- Matrix channel（mautrix-go 分叉搬移）
- A2A task handler（adk-go 内置）
- 嵌入模型（ALL_MINILM_L6_V2，ONNX 本地）
- 分类器（规则预筛 + LLM 复核）
- Nomad 分发 jobspec
