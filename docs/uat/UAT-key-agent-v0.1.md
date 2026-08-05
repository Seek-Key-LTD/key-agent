# Key Agent — UAT 验收方案

**版本**: v0.1  
**日期**: 2026-08-04  
**项目**: github.com/Seek-Key-LTD/key-agent (Key Agent 后端适配)

---

## 1. 验收目标

验证 k-agent 在 Nomad 集群上的端到端运行能力，确认三层记忆架构可工作。

## 2. 环境

- **Nomad 集群**: 当前 Nomad 节点
- **Oracle ADB**: lake5
- **二进制源**: `oca/21579-lhhq-164014/k-agent/k-agent-linux-amd64`
- **Consul KV**: `picooraclaw/dsn`

## 3. 前置条件

- [ ] DDL 已执行 (`docs/sql/PicoOracle-DDL.sql` 在 lake5 创建 PICO_SESSION / PICO_MEMORY_DEEP / PICO_MEMORY_SHARED 三表)
- [ ] Consul KV `picooraclaw/dsn` 已配置
- [ ] Nomad jobspec 已 TF apply
- [ ] k-agent 二进制可从 S3 下载 (`mc cp oca/.../k-agent-linux-amd64 - | file -`)

## 4. 测试用例

### UAT-01: 二进制可执行

**操作**:
1. `mc cp oca/21579-lhhq-164014/k-agent/k-agent-linux-amd64 /tmp/k-agent`
2. 确认 `/usr/local/bin/k-agent` 存在且可执行
3. 确认 `file /usr/local/bin/k-agent` 显示 ELF x86-64

**通过标准**: `test -x /usr/local/bin/k-agent` exit 0

### UAT-02: Oracle 连接正常

**操作**:
1. 设置环境变量：`AGENT_NAME`、`ORACLE_DSN`、`LLM_API_KEY`、`LLM_BASE_URL`、`LLM_MODEL`
2. 运行 `k-agent --agent test-agent`
3. 全部填 dummy → 启动正常, 打印 Key Agent 信息

**通过标准**: 错误消息符合预期, 正常启动时日志含 "Key Agent"

### UAT-03: 三层记忆读写

**操作**:
1. 发送消息 → 写入 PICO_MEMORY_SHARED
2. 查表 `SELECT * FROM PICO_MEMORY_SHARED WHERE AGENT_NAME='test-agent'` → 有记录
3. 发送上下文相关消息 → 命中 PICO_MEMORY_DEEP embedding 检索

**通过标准**: 写入和检索均返回正确结果

### UAT-04: Session CRUD

**操作**:
1. Create(app="picooracle", user="test-user") → 返回 session
2. Get(session_id) → 返回相同 session
3. Update(session_id, state) → 更新成功
4. List(app="picooracle", user="test-user") → 列表含刚创建的 session
5. Delete(session_id) → 删除成功

**通过标准**: 增删改查全部通过

### UAT-05: Nomad 部署

**操作**:
1. TF apply nomad job
2. `nomad status key-agent` 显示 1 个 healthy allocation
3. `nomad logs -latest key-agent` 日志正常
4. 访问 agent 端口 → 响应正常
5. TF destroy → task 自动清理

**通过标准**: 部署、运行、清理全部通过