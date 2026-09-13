# NOMAD-LABELS.md — 调度标签与话题准入

> 本文定义 Nomad 调度层需要的**节点标签**，以及**话题准入（compliance policy）**的设计。
> 设计原理见 [`AGENTS.md`](../AGENTS.md) §1（同核 + 异挂 = 可比的差异）。

---

## 0. 一句话

**邀请别人上茶台桌前，你得先知道别人方不方便聊这个事情。**

**这不是审查，是知情同意。** 而且它恰好也是工程上正确的做法：

| 做法 | 后果 |
|---|---|
| **不先问就派活** | 对方被警告 / 被拒答 → **掉线** → 可用性下降 |
| **先问再派** | 不派给它 → **它不掉线** → 可用性上升 |

**所以"礼貌"在这里等于"可靠"。**

---

## 1. 四类标签

每个节点在调度器里带一张 **agent card**。身份字段已在做（`feat(atoa): AgentCard 吐出 ASN 自定义字段`），下面把**能力与约束**补齐。

```yaml
agent_card:
  # ① 身份（已有）
  node_id:            node-sg-01
  soul_name:          锁侠
  evm_address:        0x...
  constellation:      ...
  coherence:          ...

  # ② 模型
  model_provider:     local | anthropic | google | openai
  model_id:           deepseek-v4-flash
  model_version:      ...
  endpoint:           http://100.121.16.28:4000/v1

  # ③ 合规（关键）
  jurisdiction:       sg
  provider_policy:    local-own | anthropic-aup | google-genai-policy
  topic_admission:    asn-default        # 引用话题策略名，不内联内容
  data_residency:     sg-only

  # ④ 运行
  quota_remaining:    ...
  rate_limit_state:   ok | throttled | banned
  health:             alive | degraded | dead
```

**`topic_admission` 引用一个策略名，不内联策略内容**——策略要能集中更新，不能散在每个 job 里。

---

## 2. 四道过滤

任务进来 → 逐层筛节点：

```
能力满足？      model 支持这个任务
  → 合规满足？   topic_admission 允许这个话题      ← 上桌前先问
    → 辖区满足？ data_residency 允许
      → 健康？   quota 够、没被限流
        → 派发
```

**任何一层不过就换节点——这就是 always up。**

**关键：第 2、3 层是"先问能不能，再问想不想"。** 只看第 1、4 层的调度器，一定会把任务派给一个"不该接这个任务的节点"。

---

## 3. Compliance policy —— 话题准入

### 3.1 两层结构（复用同核/异挂）

| 层 | 是什么 | 同核还是异挂 |
|---|---|---|
| **话题表** `topics.yaml` | 话题分类（**核心**） | **同核**——所有节点用同一张表，否则无法匹配 |
| **准入名单** | 每个节点方便聊哪些（**外挂**） | **异挂**——每个节点不同 |

**同核的话题表 + 异挂的准入名单 = 可比的差异。**

**而这个差异本身就是数据**：同一个话题，不同节点答不答、怎么答——**这就是可对账的东西**。

### 3.2 双重同意

**上桌前要问两次：**

1. **调度器侧**：查标签 → 只邀请"方便"的节点
2. **节点侧**：被邀请后，节点**仍可拒一次**（因为它有上下文，标签是静态的）

**第二次拒绝不是失败，是合法输出。**

### 3.3 拒答是一种合法结果

**不要把"拒答"记成错误。** 它是知情同意的正常出口，而且它本身是信息：

- 拒答 → 该节点对这个话题**没有观测条件** → 这跟"说错了"是两回事
- **所以拒答要记进账，但记的是"没答"，不是"答错"**

> **三种情况必须分开：**
> **① 不方便聊（拒答）　② 聊了但证据不足（降档）　③ 聊错了（与账不符）**
> **前两种是位置/条件问题，第三种是人的问题。处理方式完全相反。**

### 3.4 为什么必须做

**它同时解决三件事：**

1. **可用性**——不问就派 → 对方被警告 → 掉线
2. **诚实**——不方便聊的话题不硬聊，避免为了"有输出"而编
3. **可对账**——准入状态的差异本身是一张表

---

## 4. Nomad job spec 示例

```hcl
job "asn-node" {
  group "agent" {
    task "key-agent" {
      meta {
        soul_name        = "锁侠"
        model_provider   = "local"
        model_id         = "deepseek-v4-flash"
        jurisdiction     = "sg"
        provider_policy  = "local-own"
        topic_admission  = "asn-default"
        data_residency   = "sg-only"
      }
    }
  }
}
```

**出网白名单**（见 `AGENTS.md` §3.1）：只准连 LiteLLM / GBrain / Authentik。**白名单和本文的"辖区 + 数据驻留"是同一张表的两个视角**——建议合并到 `docs/ADAPTER-REGISTRY.md` 的 `出网目标` 一列。

---

## 5. 自检

```bash
# ① 每个节点都有完整的四类标签吗
nomad node status -verbose | grep -E 'soul_name|model_id|jurisdiction|topic_admission'

# ② 有没有节点缺 topic_admission（缺了就等于"什么都聊"，最危险）
#    期望：无输出
nomad job inspect asn-node | grep -L 'topic_admission'

# ③ 拒绝记录有没有被误记成错误
grep -c 'REFUSED' logs/agent.log   # 应该被单独计数，不进 error 计数
```

---

## 6. 待办

- [ ] 定义 `topics.yaml` 的第一版话题分类
- [ ] 定义 `asn-default` 准入名单的内容
- [ ] 把 `topic_admission` 的拒绝事件接进账本（记"没答"，不记"答错"）
- [ ] 与 `docs/ADAPTER-REGISTRY.md` 合并"出网目标 / 辖区 / 数据驻留"
