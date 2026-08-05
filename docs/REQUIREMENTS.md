# Key Agent — 开发需求规格 (Requirements)

> 版本: v0.2 | 日期: 2026-08-04 | 维护: manager (nuc)
> 状态: **需求定稿，可交付开发**

---

## 1. 项目定位

**Key Agent** 是一个 Go 原生的 Agent Runtime，fork 自 google/adk-go（Apache 2.0）。
核心原则：**只做加法** —— ADK-Go 是 SDK 基座，一切业务能力（记忆、渠道、技能、部署）都是可替换 adapter，不绑定任何云厂商。

```
Key Agent
  ├── SDK 基座: google/adk-go (fork, 上游同步 + patches)
  ├── LLM:     LiteLLM proxy → OpenAI 兼容端点 (sensenova deepseek-v4-flash)
  ├── Session: 短期会话 (ADK In-Memory, 未来可换 PG)
  ├── 记忆:    GBrain MCP (PG + pgvector + Voyage 1024d) — 第一集成目标
  │            └── Search / Put / Get 三操作闭环
  ├── A2A:     身份走德国 Authentik (OIDC), 协议走 ADK 内置 a2a-go
  └── 分发:    GH Actions 编译 → OCA S3 (教育网) + CF Worker (公网入口) → Nomad
```

## 2. 当前状态 (已完成)

| 模块 | 路径 | 状态 |
|------|------|------|
| Oracle memory adapter | `memory/oracle/` | ✅ 可用（已降级为可选 adapter，非默认） |
| Oracle session adapter | `session/oracle/` | ⚠️ CLOB 绑定待修 |
| k-agent 示例 | `examples/k-agent/` | ✅ LLM 调用通过（chat/completions） |
| CI flow (5 架构) | `.github/workflows/build-k-agent.yml` | ✅ 全绿 |
| Infisical secrets | project `secret-management` | ✅ 同步到 GH Actions |
| GBrain MCP 连通性 | 本机 CLI 验证 | ✅ put/search/query/get 全通 |

## 3. 待开发 (按优先级)

### P0: GBrain MCP adapter（第一集成目标）

**目标**: key-agent 通过 ADK-Go 的 `mcptoolset` 接入 GBrain MCP server。

- Transport: `mcp.CommandTransport{Command: exec.Command("gbrain", "serve")}` (stdio, 免认证)
- GBrain 已暴露 92 个 MCP tool (search/query/get_page/put_page/list_pages/tags/links/timeline)
- 集成点: `tool/mcptoolset` + `llmagent` 的 Toolsets 字段

**验收**:
1. `gbrain search <query>` 语义搜索返回结果
2. `gbrain put <slug>` 写入精选记忆（只收高质量内容：记忆卡/任务结论/已确认事实，**不灌流水账**）
3. `gbrain get <slug>` 读回完整内容
4. 关键约束: **不写每轮原始 session 到 GBrain**（GBrain 纪律=只收人工筛选内容）

**参考**: `examples/mcp/main.go`（ADK 自带 MCP 示例）

### P1: 记忆层重构

```
Session       = ADK 自己管 (短期)
GBrain        = 长期公共知识 / 可检索沉淀 (通过 MCP)
PrivateMemory = 暂缓 (有真实私密需求再做)
Neo4j         = 暂缓 (GBrain 图谱接通后再接)
Oracle        = 保留为实验 adapter, 不是路线前提
```

### P2: A2A + Authentik 身份层

- 德国 Authentik (192.168.1.199, https://authentik.capitaltrain.cn) — **已修复上线**
- 角色: OIDC issuer (短期可信登录), 不替代 DID/钱包
- 分层:
  - Authentik = "这次请求是谁发的" (OIDC workload identity)
  - Key Agent = "能调用哪个 agent/skill/task" (业务授权)
  - DID/EVM = 链上长期身份 (后置 registry, 不做进第一版)
- 落地: Authentik 建 provider `key-agent-a2a`, 每 agent 一个 client, client_credentials → JWT → A2A Bearer
- 4 个 scope: `a2a.invoke / a2a.task.read / a2a.task.cancel / a2a.card.read`

### P3: 渠道

- Matrix: 已有 mautrix-go 分叉经验, 后续搬
- Gitea webhook: 走 A2A task handler, 不另起 HTTP webhook

## 4. 基础设施状态

| 资源 | 地址 | 状态 |
|------|------|------|
| LiteLLM (MBP) | http://100.121.16.28:4000/v1 | ✅ 已修复 (DATABASE_URL 恢复) |
| GBrain | gbrain serve (stdio) / :48899/mcp | ✅ 运行中 |
| Oracle ADB lake5 | 见 Consul KV `picooraclaw/dsn` | ✅ 表已建 |
| PG 集群 | 192.168.31.201/.203/.204 | ✅ Patroni |
| Authentik | https://authentik.capitaltrain.cn | ✅ 刚修复 (PG 202 + Redis 104) |
| Infisical | https://infisical.git4ta.fun | ✅ |
| OCA S3 (教育网) | oca/21579-lhhq-164014/ | ✅ |
| CF Worker | cernet-s3.git4ta.fun | ✅ |

## 5. 纪律与约束

1. **只做加法**: 不重构 ADK-Go 核心, 新能力=新 adapter
2. **文本是本金**: embedding 是可再生索引, 必须存 model_id/dimension/version
3. **不同 embedding 模型不混索引**: 查询按 collection 的 embedding family 路由
4. **GBrain 只收精选**: 记忆卡/结论/事实, 不灌流水账
5. **不改上游公共 API**: 保持 ADK-Go 兼容
6. **不绑定云厂商**: Oracle/Neo4j/云 embedding 都是 adapter, 不是前提

## 6. 快速上手

```bash
# 1. clone
git clone https://ghpx.git4ta.fun/github.com/Seek-Key-LTD/adk-go.git ~/Projects/github/adk-go
cd ~/Projects/github/adk-go

# 2. 本地跑 k-agent (验证 LLM + Oracle 连通)
./dist/k-agent-linux-amd64 \
  -dsn "$(curl -s http://127.0.0.1:8500/v1/kv/picooraclaw/dsn | jq -r '.[0].Value' | base64 -d)" \
  -base-url "http://100.121.16.28:4000/v1" \
  -secret "$(cat ~/.infisical/token | head -c 40)" \
  -model "azure-deepseek-v4-flash" \
  -prompt "你好, 测试"

# 3. 测 GBrain MCP
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | gbrain serve

# 4. 构建
go build ./...
```

## 7. 验收矩阵 (UAT)

| ID | 内容 | 通过标准 |
|----|------|---------|
| UAT-01 | 二进制分发 | OCA + CF Worker 都能拉到 |
| UAT-02 | 参数校验 | dsn/base-url/secret 缺一报错 |
| UAT-03 | GBrain 写入 | put 成功, chunks>0 |
| UAT-04 | GBrain 搜索 | 写入的内容能搜到且排名前3 |
| UAT-05 | LLM 调用 | chat/completions 返回内容 |
| UAT-06 | 记忆闭环 | 写→搜→读 全通 |
| UAT-07 | A2A 认证 | Authentik JWT 能通过验签 |
| UAT-08 | Nomad 分发 | job 拉起, 健康检查绿 |
