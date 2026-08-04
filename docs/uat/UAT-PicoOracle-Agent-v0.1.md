# PicoOracle Agent — UAT 验收方案

**版本**: v0.1  
**日期**: 2026-08-04  
**项目**: github.com/Seek-Key-LTD/adk-go (PicoOracle 后端适配)

---

## 1. 验收目标

验证 oracle-agent 在 Nomad 集群上的端到端运行能力，确认三层记忆架构可工作。

---

## 2. 验收环境

| 组件 | 说明 |
|------|------|
| Nomad 集群 | PVE × 4 + PBS × 1, dc1, region=global |
| 测试节点 | pve (x86_64), 任选一个 |
| 二进制源 | `oca/21579-lhhq-164014/picooracle/oracle-agent-linux-amd64` |
| 配置源 | Consul KV `picooraclaw-agent/*` |
| Oracle DB | ADB lake5, 需先建 PICO_* 表 |
| LLM | litellm.capitaltrain.cn/v1 |

---

## 3. 前置条件 Checklist

- [ ] DDL 已执行 (`docs/sql/PicoOracle-DDL.sql` 在 lake5 创建 PICO_SESSION / PICO_MEMORY_DEEP / PICO_MEMORY_SHARED 三表)
- [ ] 向量索引已建 (`ALTER TABLE ... ADD VECTOR INDEX`)
- [ ] Consul KV `picooraclaw-agent/` 下已写入：ORACLE_DSN, OPENAI_BASE_URL, LITELLM_API_KEY, ORACLE_MEMORY_KEY, AGENT_NAME, MODEL_ID
- [ ] Nomad jobspec 已 TF apply (`nomad_job.picooracle_agent`)
- [ ] 测试节点上有 `mc` 且配置了 oca alias
- [ ] oracle-agent 二进制可从 S3 下载 (`mc cp oca/.../oracle-agent-linux-amd64 - | file -`)

---

## 4. 验收用例

### UAT-01: 二进制分发验证
**步骤**:
1. 在 Nomad task 启动日志中确认 `mc cp` 成功拉取
2. 确认 `/usr/local/bin/oracle-agent` 存在且可执行
3. 确认 `file /usr/local/bin/oracle-agent` 显示 ELF x86-64

**预期**: 日志无 ERROR, 健康检查通过

**通过标准**: `test -x /usr/local/bin/oracle-agent` exit 0

---

### UAT-02: 参数校验
**步骤**:
1. 缺 ORACLE_DSN → `log.Fatal("-dsn required")`
2. 缺 OPENAI_BASE_URL → `log.Fatal("-base-url required")`
3. 全部填 dummy → 启动正常, 打印 PicoOracle Agent 信息

**通过标准**: 错误消息符合预期, 正常启动时日志含 "PicoOracle Agent"

---

### UAT-03: memory.Service — AddSessionToMemory
**步骤**:
1. 构造一个含 3 条 user/assistant 对话的 session.Session
2. 调用 `AddSessionToMemory(ctx, session)`
3. 查 PICO_MEMORY_DEEP / PICO_MEMORY_SHARED 表

**通过标准**:
- Session 内容被写入正确的层（根据 classifier 规则）
- 384-dim embedding 向量存入 EMBEDDING 列
- METADATA_JSON 含 layer/agent_name/timestamp
- DEEP 层内容已 AES-256-GCM 加密（content_plaintext 为 NULL）

---

### UAT-04: memory.Service — SearchMemory
**步骤**:
1. 用与 UAT-03 相似内容的 query 调用 `SearchMemory`
2. 返回结果集

**通过标准**:
- 返回 N >= 0 条匹配结果
- 结果按 VECTOR_DISTANCE 排序
- 结果含 agent_name + content

---

### UAT-05: session.Service — CRUD
**步骤**:
1. Create(app="picooracle", user="test-user") → 返回 session
2. AppendEvent(session, event) → 更新 state
3. Get(session_id) → 取回完整 session
4. List(app="picooracle", user="test-user") → 列表含刚创建的 session
5. Delete(session_id) → 表记录删除

**通过标准**: 全部 5 操作成功, 数据一致

---

### UAT-06: 三层记忆隔离
**步骤**:
1. 发一条含"彭罗斯错了"内容的 session → 应进 PICO_MEMORY_DEEP（个人）
2. 发一条含通用知识内容的 session → 应进 PICO_MEMORY_SHARED
3. 发一条含即时对话内容的 session → 应进 PICO_SESSION
4. 跨 agent 搜索 → DEEP 内容仅当前 agent 可见, SHARED 内容所有 agent 可见

**通过标准**: 分类正确, 隔离符合预期

---

### UAT-07: Nomad 集群集成
**步骤**:
1. TF apply `nomad_job.picooracle_agent`
2. `nomad status picooracle-agent` 显示 1 个 healthy allocation
3. `nomad logs -latest picooracle-agent` 日志正常
4. 修改 Consul KV 配置 → task 重启后生效
5. TF destroy `nomad_job.picooracle_agent` → task 自动清理

**通过标准**: 0 failed allocations, 健康检查持续 green

---

### UAT-08: 并发 & 稳定性
**步骤**:
1. 并发 5 个 AddSessionToMemory 调用
2. 持续运行 5 分钟

**通过标准**: 无 goroutine leak, 无 panic, 结果数 = 5

---

## 5. 通过/失败判定

| 用例 | 权重 | 判定 |
|------|------|------|
| UAT-01 二进制分发 | 必须 | 阻塞 |
| UAT-02 参数校验 | 必须 | 阻塞 |
| UAT-03 AddSessionToMemory | 必须 | 阻塞 |
| UAT-04 SearchMemory | 必须 | 阻塞 |
| UAT-05 Session CRUD | 必须 | 阻塞 |
| UAT-06 三层隔离 | 重要 | 非阻塞 |
| UAT-07 Nomad 集成 | 必须 | 阻塞 |
| UAT-08 稳定性 | 参考 | 非阻塞 |

**阻塞用例全部通过 = UAT 通过**

---

## 6. 验收记录

| 用例 | 结果 | 操作人 | 日期 | 备注 |
|------|------|--------|------|------|
| UAT-01 | ⬜ | | | |
| UAT-02 | ⬜ | | | |
| UAT-03 | ⬜ | | | |
| UAT-04 | ⬜ | | | |
| UAT-05 | ⬜ | | | |
| UAT-06 | ⬜ | | | |
| UAT-07 | ⬜ | | | |
| UAT-08 | ⬜ | | | |

---

## 7. 阻塞项 & 风险

- **DDL 未执行**: UAT-03/04/05/06 依赖 Oracle 表, 需先跑 DDL
- **AES-256 key**: UAT-03 DEEP 层加密依赖 ORACLE_MEMORY_KEY 已设置
- **Litellm 配额**: UAT 期间消耗 embedding + LLM 调用, 确认额度
- **Consul KV 渲染**: UAT-07 依赖 consul-template 在 Nomad task 启动前渲染 .env
