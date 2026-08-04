# Key Agent

> 基于 Google ADK-Go 的 Go 原生 Agent Runtime — 只做加法，最小核心 + 可替换 adapter。

这是 Seek-Key 的 agent 运行时，fork 自 [google/adk-go](https://github.com/google/adk-go)（Apache 2.0）。上游同步保持兼容，在此之上加自己的 adapter 和产品层。

## 设计纪律

1. **ADK-Go 是 SDK 基座**，不是产品本体
2. **Memory / Session / Channel / Skill 都是接口**，具体后端是可替换 adapter
3. **文本是本金，embedding 是可再生索引**
4. **不同 embedding 模型的向量不能混在同一索引**
5. **不绑定任何云厂商** — Oracle / PG / Neo4j 都是 adapter，不是前提

## Adapter 路线

| 层 | 默认 | 可选 |
|----|------|------|
| Embedder | 本地 384 维 (ALL_MINILM_L6_V2 ONNX) | Voyage AI、Oracle 内置 |
| Memory Store | PostgreSQL + pgvector | Oracle ADB (lake5)、GBrain |
| Session | In-Memory | Oracle、PG |
| LLM | LiteLLM proxy (sensenova deepseek-v4-flash) | 任意 OpenAI 兼容端点 |
| Channel | — | Matrix (mautrix-go)、Gitea webhook |
| Distribution | Nomad | — |

## 已实现

| 模块 | 路径 | 状态 |
|------|------|------|
| Oracle memory（三层+加密+向量） | `memory/oracle/` | ✅ 可用（adapter，非默认） |
| Oracle session CRUD | `session/oracle/` | ⚠️ CLOB 绑定待修 |
| oracle-agent 示例 | `examples/oracle-agent/` | ✅ LLM 调用通过 |
| DDL | `memory/oracle/ddl.sql` | ✅ 已在 lake5 执行 |
| CI flow（5 架构编译） | `.github/workflows/build-oracle-agent.yml` | ✅ 全绿 |
| UAT 方案 | `docs/uat/` | ✅ UAT-01~04 通过 |

## 白嫖的艺术

| 资源 | 用法 | 成本 |
|------|------|------|
| Oracle ADB (4 lake + 2 river) | 向量检索 + embedding | 免费额度内 |
| PG 集群 (.201/.203/.204) | pgvector 默认后端 | 自有 |
| 本地 384 维 ONNX embedding | 默认 embedder | 零 |
| GH Actions | CI 编译 5 架构 | 免费 |
| OCA S3 (教育网) | 二进制分发 | 免费 |
| CF Worker (cernet-s3) | 公网入口 | 免费 |

## Secrets 管理

- Infisical project: `secret-management` (ID: `349cc5f0-e13b-446e-a940-18a7c671146a`)
- 自动单向同步到 GitHub Actions secrets
- Vault / OpenBao 做运行时密钥（已就绪）

## 开发状态

详见 [DEV-STATUS.md](docs/DEV-STATUS.md)
