# ADAPTER-REGISTRY.md — 外挂登记表

> **规矩**：每加一个外挂（adapter），必须在本表登记一行。
> **没有登记的外挂，不许进 main。** 见 [`AGENTS.md`](../AGENTS.md) §2。
>
> 登记的目的是让差异**可复算**：两个节点跑同一份核心，外挂不同——但每一个外挂都是可见、可查、可替换的。

---

## 登记字段

| 字段 | 说明 |
|---|---|
| **外挂名** | 目录或包路径 |
| **类型** | L1 Tool / L2 Toolset / L3 Plugin / L4 Agent（四道合法门，见 AGENTS.md §2） |
| **后端** | 实际连的是什么 |
| **model_id** | 若涉及 embedding / LLM，写明模型标识 |
| **dimension** | 若涉及向量，写明维度 |
| **version** | 后端或协议的版本 |
| **出网目标** | 这个外挂会连到哪里（用于出网白名单，见 AGENTS.md §3.1） |

---

## 当前登记

### LLM

| 外挂名 | 类型 | 后端 | model_id | dimension | version | 出网目标 |
|---|---|---|---|---|---|---|
| `model/openaimodel`（LiteLLM proxy） | L4 | LiteLLM | `azure-deepseek-v4-flash` | — | — | `http://100.121.16.28:4000/v1` |
| `model/gemini` | L4 | Google GenAI | `gemini-2.5-flash` | — | — | `generativelanguage.googleapis.com` |

### Memory

| 外挂名 | 类型 | 后端 | model_id | dimension | version | 出网目标 |
|---|---|---|---|---|---|---|
| `memory/oracle` | L2 | Oracle ADB (lake5) | — | 依 embedding | — | Oracle ADB |
| `memory/pgvector` | L2 | PostgreSQL + pgvector | — | 依 embedding | — | `192.168.31.201/.203/.204` |
| GBrain（via MCP） | L2 | PostgreSQL + pgvector | `voyage-*` | **1024** | — | `gbrain serve` (stdio) / `:48899/mcp` |

### Embedder

| 外挂名 | 类型 | 后端 | model_id | dimension | version | 出网目标 |
|---|---|---|---|---|---|---|
| 本地 ONNX | L1 | ONNX Runtime | `ALL_MINILM_L6_V2` | **384** | — | **无（本地）** |
| Voyage AI | L1 | Voyage API | `voyage-*` | **1024** | — | `api.voyageai.com` |

> ⚠️ **不同 embedding 模型的向量不能混在同一索引**。查询按 collection 的 embedding family 路由。见 `docs/REQUIREMENTS.md` §5。

### Session

| 外挂名 | 类型 | 后端 | model_id | dimension | version | 出网目标 |
|---|---|---|---|---|---|---|
| `session`（In-Memory） | L2 | 进程内存 | — | — | — | 无 |
| `session/oracle` | L2 | Oracle ADB | — | — | ⚠️ CLOB 绑定待修 | Oracle ADB |

### Channel

| 外挂名 | 类型 | 后端 | model_id | dimension | version | 出网目标 |
|---|---|---|---|---|---|---|
| `messaging/matrix.go` (Matrix Sync & Pager) | L2 | Matrix Homeserver (Tuwunel) | — | — | v3 | `https://matrix.git4ta.fun` |
| `messaging/mastodon.go` (Mastodon AP Channel) | L2 | Mastodon ActivityPub | — | — | v1/v2 | `https://mastodon.capitaltrain.cn` |
| Gitea webhook | L2 | Gitea | — | — | — | 走 A2A task handler，不另起 HTTP |

### 身份 / 分发

| 外挂名 | 类型 | 后端 | model_id | dimension | version | 出网目标 |
|---|---|---|---|---|---|---|
| Authentik（OIDC） | L3 | Authentik | — | — | — | `https://authentik.capitaltrain.cn` |
| Nomad | L4 | Nomad | — | — | — | Nomad server |

---

## 待办

- [ ] 每个 adapter 补 `version`
- [ ] 补 `evm/`（链上身份 / 结算）的登记
- [ ] 补 `artifact/`（IPFS pinning）的登记
- [ ] 出网白名单汇总成一份可执行的清单
