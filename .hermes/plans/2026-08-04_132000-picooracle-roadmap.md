# PicoOracle Agent — 6 个工作包路线图

> **For Hermes:** 每个工作包是一张可直接分派的 Agent 工单；包内 checklist 由同一责任 Agent 顺序完成，不拆成 21 个并行 Agent。

**目标：** 在不修改 ADK-Go 公共接口的前提下，把已有 Oracle prototype 变成正确、可测、可部署的 PicoOracle runtime；之后再评估完整性承诺、链上清算与渠道集成。

**当前状态：** `memory/oracle`、`session/oracle`、`examples/oracle-agent`、DDL、构建 CI 和 UAT 已存在。现在不是从零搭架构，而是先把 adapter 做对。

## 调度规则

只开 6 张工单，不把 14 个节点硬塞满。前两个阶段实际只允许 2–3 个并行包；后续包有明确前置，不抢跑。

```text
包 A：契约 + Schema ───────────┬─ 包 B：Session adapter
                               └─ 包 C：Memory + Crypto
包 B + C ──────────────────────┬─ 包 D：运行时 + UAT + CI + Nomad
                               └─ 包 E：完整性承诺研究（可选）
包 D ──────────────────────────── 包 F：A2A、链上缓存、Skills、Matrix（后续）
```

| 工单 | 责任边界 | 前置 | 初始是否开工 |
|---|---|---|---|
| A | 架构约束、迁移、权限与审计边界 | 无 | 是 |
| B | Oracle session adapter | A 的 schema 决策 | 等 A 的表结构定稿 |
| C | Oracle memory、分类、加密 | A 的 schema 决策 | 等 A 的表结构定稿 |
| D | 生产 launcher、UAT、CI、Nomad | B、C | 否 |
| E | Merkle / 访问可审计性研究 | A | 可选，不阻塞 D |
| F | A2A、链上缓存、skills、Matrix | D | 否 |

---

## 工作包 A — 契约、Schema 与真实安全边界

**目标：** 冻结正确的数据模型与安全术语，给 B/C 一个稳定接口；不写钱包、不接 Matrix。

**文件：**
- 新建 `docs/picooracle/architecture.md`
- 新建 `docs/picooracle/threat-model.md`
- 新建 `memory/oracle/migrations/0001_initial_schema.sql`
- 新建 `memory/oracle/migrations/0002_session_events.sql`
- 新建 `memory/oracle/migrations/README.md`
- 修改 `memory/oracle/ddl.sql`
- 新建 `memory/oracle/migrations/0004_access_audit.sql`

**完成定义：**

- 版本化、可重跑的迁移替代单一手工 DDL；`PICO_SESSION_EVENT` 用于有序事件，不再依赖 JSON 字符串拼接。
- 明确 session / deep / shared 的读写权限；shared 只能显式发布，不能由关键词碰巧升级。
- 深层 embedding 被列为敏感数据，不能声称“无模型权重就不会泄露”。
- SELECT 审计使用 Oracle FGA/Unified Audit；**禁止**设计 `AFTER SELECT` 触发器。
- “非观测证明”改称“应用层可审计访问”；DBA、宿主机、备份及控制平面不在该保证内。

**验收：**

```bash
# 对独立 Oracle 测试 schema 执行；连续执行两遍。
# 第二遍只能显示已应用版本，不能破坏数据或失败。
<migration-runner> status
<migration-runner> apply
<migration-runner> apply
```

**提交：** `feat(picooracle): add versioned schema and audit boundary`

---

## 工作包 B — Session adapter 正确性

**目标：** 让 `session/oracle` 真正满足 ADK `session.Service`，先把已有实现的安全和持久化缺陷修完。

**文件：**
- 修改 `session/oracle/oracle_session.go`
- 新建 `session/oracle/oracle_session_integration_test.go`
- 新建 `session/oracle/testdata/README.md`
- 参考 `session/sessiontestsuite/service_suite.go`

**必须完成：**

1. 创建 opt-in Oracle integration test：`oracle_integration` build tag + `ORACLE_TEST_DSN`；未设置时明确 skip。
2. 修复事件持久化：`AppendEvent` 写 `PICO_SESSION_EVENT`，同一事务更新 state/time；`Get` 重新组装 events，并支持 `NumRecentEvents`、`After`。
3. 修复权限：`Get`、`Delete`、所有 update 必须用 `app_name + user_id + session_id` 限定。
4. 修复 `LastUpdateTime()`；所有 JSON/rows error 必须返回。
5. 跑现有 session conformance suite 中适用案例：create、duplicate、get/list/delete、event round-trip、state、跨用户拒绝。

**验收：**

```bash
go test ./session/oracle/...                 # 无测试 DSN 时 clean skip
go test -tags=oracle_integration ./session/oracle/...
```

跨 user 的 get/delete 必须失败；追加两个 event 后读回顺序、内容、更新时间一致。

**提交：** `fix(session/oracle): make session service conformant`

---

## 工作包 C — Memory + 分类 + 加密

**目标：** 让 Oracle memory 有可用检索、深层隔离和可靠的加密边界。这个包由一个 Agent 负责，避免 schema/crypto/search 交叉踩文件。

**文件：**
- 修改 `memory/oracle/oracle_memory.go`
- 新建 `memory/oracle/oracle_memory_integration_test.go`
- 新建 `memory/oracle/keyprovider.go`
- 新建 `memory/oracle/crypto.go`
- 新建 `memory/oracle/crypto_test.go`
- 修改 `memory/classifier/classifier.go`
- 新建 `memory/classifier/classifier_test.go`
- 新建 `memory/oracle/migrations/0003_deep_memory_key_version.sql`

**必须完成：**

1. 用注入的确定性 test embedder 写 integration tests；禁止外部 LLM 调用。
2. `SearchMemory` 分别检索 shared 和 caller 自己的 deep，解密 deep 后填充真正的 `memory.Entry` 内容/metadata；不得只返回 Author。
3. 对 deep query 加 agent ownership predicate；定义跨层排序、limit 和空结果语义。
4. key provider 要验证精确 32 字节 hex key，错误立即失败；不能默默生成零填充 key。
5. AES-GCM 绑定 memory ID、owner、schema/key version 作为 AAD；删除无实际 KDF 用途的 `salt`，或赋予清晰含义。
6. deep 默认；shared 写入必须带显式可信发布授权。分类器只能建议，不能自行越权发布。

**验收：**

```bash
go test ./memory/classifier/...
go test -tags=oracle_integration ./memory/oracle/...
```

- Agent A 能读自己的 deep + allowed shared，不能读 Agent B 的 deep。
- 篡改 ciphertext/nonce/AAD 后必须解密失败。
- 用户文本包含 shared keyword 时仍不能自动进 shared。

**提交：** `feat(memory/oracle): add secure two-layer retrieval`

---

## 工作包 D — 可运行 agent、UAT、CI、Nomad

**目标：** 将“能连数据库的 example”升级为真实可交付运行时；只在 B/C 全绿后启动。

**文件：**
- 新建 `cmd/picooracle-agent/main.go`
- 新建 `cmd/picooracle-agent/config.go`
- 新建 `cmd/picooracle-agent/config_test.go`
- 修改 `examples/oracle-agent/main.go`
- 修改 `docs/uat/UAT-PicoOracle-Agent-v0.1.md`
- 修改 `.github/workflows/build-oracle-agent.yml`
- 新建 `scripts/picooracle/verify-schema.sh`
- 新建 `scripts/picooracle/run-uat.sh`
- 新建 `deploy/nomad/picooracle-agent.nomad.hcl`
- 新建 `deploy/nomad/README.md`

**必须完成：**

1. `cmd/picooracle-agent` 构造真实 ADK runner；example 保持为连接/最小 smoke，不再假称完成 UAT-03/04。
2. 配置验证早于数据库/网络访问；DSN、key、base URL 在日志中脱敏。
3. UAT 拆为：离线 config、isolated Oracle integration、Nomad staging；记录 schema version、binary SHA 和结果证据。
4. CI 使用模块声明的 Go 1.26.5；用 `go mod tidy -diff`，不在 CI 静默改 `go.mod`；build/test/lint 通过才上传二进制。
5. Nomad 只部署版本化且带 checksum 的 artifact，使用最小权限短期凭据；没有生产密钥进 Consul KV 或日志。

**验收：**

```bash
go build -mod=readonly work
go test -race -mod=readonly -count=1 -shuffle=on work
golangci-lint run
go mod tidy -diff
go run ./cmd/picooracle-agent --help
```

Staging Nomad allocation 校验二进制 hash、成功 readiness、可回滚；只有此后才标记 UAT-01/02/07 通过。

**提交：** `feat(picooracle): ship tested Oracle agent runtime`

---

## 工作包 E — 完整性承诺研究（可选，不阻塞）

**目标：** 只验证 Merkle commitment 是否对应用层审计有价值，不承诺 ZKP 或绝对非观测。

**文件：**
- 新建 `internal/picooracle/merkle/tree.go`
- 新建 `internal/picooracle/merkle/tree_test.go`
- 新建 `docs/picooracle/commitments.md`
- 新建 `docs/decisions/ADR-001-picooracle-access-attestation.md`

**必须完成：**

1. 规定 canonical leaf、域分离、排序、版本和外部 witness 输入。
2. 实现 deterministic root、inclusion proof、篡改失败、回滚检测的离线测试。
3. 产出 ADR：defer / integrity-only / 另立外部 witness + attested-runtime 项目，三选一。

**验收：** 所有测试离线、可重现；文档只称“完整性 commitment/可审计访问”，不称“ZKP/DBA 未观测证明”。

**提交：** `spike(picooracle): evaluate access integrity commitments`

---

## 工作包 F — A2A 经济层与可选渠道（后续）

**目标：** 在 D 完成后，按最小价值顺序引入链上事件缓存、A2A、skills 或 Matrix；不并行开四个大型子系统。

**先决策顺序：**

1. 链上事件缓存与幂等模型；
2. 一个 testnet A2A → signer MCP → chain receipt → Oracle index 闭环；
3. 在 GitSource / MCP / A2A 三者中选择**一种** Hermes skills 分发方式；
4. 最后才移植 Matrix bridge 与 reaction。

**文件（按选择创建，不预先铺空壳）：**
- `docs/picooracle/settlement-contract.md`
- `memory/oracle/migrations/0005_chain_event_cache.sql`
- `internal/picooracle/settlement/`
- `docs/decisions/ADR-002-picooracle-skill-distribution.md`
- `internal/picooracle/skills/` **或**对应 MCP/A2A adapter
- `internal/picooracle/matrix/`

**硬约束：**

- Oracle cache 有 `chain_id`、token contract、base-unit integer amount、`block_hash`、`log_index`、idempotency key 和 `submitted/confirmed/finalized/reorged` 状态；链上才是 source of truth。
- real private key 永不出现在单测、CI 或 Agent context。
- Skills 的远端加载必须有 pin/signature/rollback；Matrix 必须有 sender filter、event 去重、rate limit，清算不得走 Matrix 广播。

**验收：** offline fake signer 覆盖重试幂等性；staging 仅使用测试币并能从 tx/event 唯一定位到 cache row。Matrix 只在 bot 自回环和重复 event 均被测试阻断后开放。

**提交：** 每个子系统独立 PR；禁止与 A–E 混合。

---

## 现在应该派谁

现在只派 **1 个 Agent 做 A**。A 的迁移/权限边界定稿后，同时派 **B 和 C**；E 若有空闲节点可独立做，但不影响交付。D 等 B/C 通过后再派；F 完全不启动。

这样是 `1 → 2(+1 optional) → 1 → 1` 的节奏，不是 21 个并发任务。 
