# RFC: 作曲家调度器与 WBS 关键路径编排架构规范
## (Composer Scheduler & WBS Critical Path Orchestration Architecture)

> **状态**：`PROPOSED / UNDER REVIEW`（通盘考虑与设计评审阶段）  
> **所属仓库**：`key-agent` (ADK Go)  
> **关联模块**：`cmd/agent_dispatcher`, `integration/calendar.go`, `integration/tea`, `workflow/dynamic_scheduler.go`, `model/`  
> **核心立意**：将多 Agent 系统的资源调度，从死板的瞬时限流器升级为**“以 Gitea Task 为 WBS 骨架、以飞书日历为时间物理占用、以 5小时滑动窗口与每周预算为双重约束的交响乐节拍规划器”**。

---

## 一、 战略背景与核心痛点

随着团队引入各类高级 AI 订阅服务（如 Claude Pro/Team、OpenAI Team、Cursor 等），这些服务均设立了严苛的**双重非线性配额紧箍咒**：
1. **短周期滑动窗口限制**：例如 **5 小时动态滚动窗口（5-Hour Rolling Window）**，对消息数或 Token 密度做严格波峰削减；
2. **长周期硬预算限制**：例如 **每周固定/滚动配额（Weekly Hard Cap）**，一旦透支，整周业务彻底熔断。

### 传统并发调度的致命缺陷：
* **脉冲式早夭（Burst Starvation）**：不受控的 Agent 会在前 15~30 分钟内并发打满 5 小时额度，导致后续 4.5 小时全线 429 报错，交互式开发被迫中断；
* **配额盲目消耗（Priority Blindness）**：低优先级的后台批量任务（如代码索引、抓取、摘要）抢光了高价值的实时决策或用户交互（HITL）额度；
* **状态易失性（Volatile Failure）**：若在 Go 内存中直接做排队，一旦服务重启或机器漂移，排队任务与滑动窗口水位数据当场蒸发。

---

## 二、 核心哲学：作曲家（Composer）与 WBS 路径规划者

交响乐绝不会在前两个小节把全曲的音符砸光，随后陷入四十分钟的尴尬死寂。

**作曲家调度器的根本定位，就是一个带有双重物理与能源约束的 WBS（工作分解结构）关键路径规划器（Critical Path Method, CPM）。**

```
                 【作曲家 (WBS 关键路径规划器) 架构拓扑】

       [ Gitea Issue: 战略总目标 (如: 某房企表外债全量穿透尸检) ]
                                 │
                                 ▼ WBS 拆解 (Markdown Checklist Tasks)
       ┌─────────────────────────────────────────────────────────┐
       │ - [ ] Task A: 股权穿透与七层SPV还原 (est: 20m, 15k tok)  │
       │ - [ ] Task B: 明股实债信托合同比对  (est: 40m, 35k tok)  │
       │ - [ ] Task C: 汇总出具司法鉴定报告  (est: 15m, 10k tok)  │
       └─────────────────────────────────────────────────────────┘
                                 │
                 ┌───────────────┴───────────────┐
                 ▼                               ▼
       【时间轴：日历物理排期】          【能源轴：5h滑动窗口与周配额】
   (Feishu Calendar Occupation)        (LLM Plan Rate Pacing)
                 │                               │
                 │ 锁定 Agent Topaz              │ 实时监测 5h 剩余配额:
                 │ 14:00 - 14:20                 │ 还能容纳 40k tokens
                 ▼                               ▼
       ┌─────────────────────────────────────────────────────────┐
       │            【作曲家 WBS 关键路径推演 (CPM)】            │
       │                                                         │
       │ 1. 识别 Task A 与 Task B 之间的 DAG 依赖拓扑            │
       │ 2. Task A 立即放行 (占用 14:00~14:20，扣除 15k tokens)  │
       │ 3. 预测 Task B 启动时将触碰 5h 窗口红线 (需 35k tokens) │
       │ 4. 触发“切调”机制：平滑路由至备用 Plan-2，或插入 10 分钟 │
       │    节拍缓冲，在飞书日历上自动顺延锁定占用               │
       └─────────────────────────────────────────────────────────┘
                                 │
                                 ▼ 放行并指派 Worker 执行
                     Agent 顺畅推进，零 429 报错
                                 │
                                 ▼
         通过 tea API 勾选 `- [x]`，释放日历占用，回填真实耗时
```

---

## 三、 基础设施落地：深度融合 Gitea / tea API

纯内存调度器在工业级生产环境中是不可接受的，必须将 **Gitea / `tea`** 作为全局唯一事实真理源（SSOT）。

### 1. 为什么“挑重点打进去”？
不直接引入完整的 `tea` CLI（避免将 Bubble Tea 等交互式终端 TUI 依赖打入后台服务），而是引入 Gitea 官方轻量 Go SDK（`code.gitea.io/sdk/gitea`），构建 `key-agent/integration/tea` 模块。

### 2. 核心对接重点：
* **重点 A：Task Checklist 状态机**
  * 原生解析与维护 Issue Body 中的 `- [ ]` 与 `- [x]`；
  * 任务完成时自动原子勾选，并在评论区附上机器可读的消耗凭证；
* **重点 B：Estimation（工时预估）双轨提取**
  * **行内标注（首选）**：`- [ ] 任务名 (est: 30m, tokens: 20k)`；
  * **Gitea 原生 Time Tracking**：读取 Issue 的 `total_track_time` 或工时预算 API；
* **重点 C：Label 乐章标记映射**
  * `priority/P0-allegro`（快板/实时交互）：零等待穿透；
  * `priority/P1-andante`（行板/主工作流）：标准调度；
  * `priority/P2-adagio`（慢板/批处理任务）：仅在 5h 窗口利用率低于 40% 的谷底期放行。

---

## 四、 飞书日历真实占用（Feishu Calendar Occupation）联动

终结现有代码中硬编码 `30 * time.Minute` 的虚假占用：

1. **从 Gitea Task 提取真实的 `est: 45m`**；
2. 调度器调用 `integration.CreateOccupationEvent`，在目标 Agent 的日历上真正锁定 `14:00 ~ 14:45`；
3. 如果 5 小时配额拥堵需要延迟放行，调度器主动更新日历事件，将占用块平滑向后推移，**让团队人类成员在飞书日历上一眼看清各个 Agent 真实的排产负荷**。

---

## 五、 特别注意事项与深水区工程陷阱（重点通盘考虑）

在后续具体立项与编码前，团队必须对以下五大陷阱达成设计共识：

### ⚠️ 1. 滑动窗口的时间漂移与时钟“安全垫”（Clock Drift & Safety Margin）
* **陷阱**：商业 Plan（如 Claude/OpenAI）的 5 小时滑动窗口是服务端的 UTC 滚动，不是绝对精确的整点，且网络延迟可能导致最后几秒的请求被判定违规；
* **防御规范**：
  * 作曲家必须保留 **8% ~ 10% 的配额安全垫（Headroom）**；
  * 窗口容量上限若为 100k，调度器算到 90k 时即视为满载，绝不压榨到 99.9% 边缘；
  * 留出的 10% 净空作为 P0 级紧急人机交互的专属直通通道。

### ⚠️ 2. 预估（Estimation）与实际消耗的方差修正（Pre-allocation vs Actual）
* **陷阱**：任务预估 10k tokens，但 LLM 进入思维发散或反思循环，实际喷出了 40k tokens，直接穿透 5h 窗口；
* **防御规范**：
  * **双阶段记账**：任务启动前在滑动窗口中“预扣（Hold）”预估额度；任务结束后以 API 实际返回的 `usage.total_tokens` 进行“真实核销（Settle）”；
  * **EWMA 动态校准**：维护历史执行系数。若某个 Agent 长期出现实际消耗高于预估 1.5 倍，调度器自动乘以动态膨胀因子。

### ⚠️ 3. 并发争抢与任务抢占的分布式互斥锁（Task Mutex）
* **陷阱**：多个 Worker Agent 或多个 Dispatcher 实例同时被唤起，同时抢走 Gitea Issue 里同一个未勾选的 Task；
* **防御规范**：
  * Agent 认领任务前，必须原子化更新 Gitea Issue：将任务打上行内占用标记（如 `- [ ] [In-Progress: Topaz] Task 1`）或打上 `status/in-progress` 标签；
  * 认领失败者自动让出并获取下一个未锁定的 Task。

### ⚠️ 4. 慢板长任务的“永久饥饿（Starvation）”防护
* **陷阱**：如果白天持续有 P0/P1 的高优先级任务涌入，P2（慢板/离线批处理）任务可能因为 5 小时窗口始终在 60% 以上而被无限期后延；
* **防御规范**：
  * 引入 **老化权重（Aging Mechanism）**：P2 任务每在 Gitea 里等待 2 小时，优先级自动递增一档，直到升级为普通任务强制放行；
  * 设置硬性截止时间（Deadline），确保即使在波峰期，离线任务也能在限定自然日内闭环。

### ⚠️ 5. 飞书日历与 Gitea 状态的一致性幂等（Idempotency Key）
* **陷阱**：网络抖动导致任务失败重试时，日历上重复生成大量同名占用块；
* **防御规范**：
  * 日历事件的幂等键直接绑定 Gitea 任务指纹：`ext_id = fmt.Sprintf("gitea-%d-task-%d", issueID, taskIndex)`；
  * 重试时先查询是否存在对应 `ext_id`，存在则更新时间区间（Update），不存在才新建（Create）。

---

## 六、 两阶段分期实施路线图

```
                【两阶段演进甘特草案】

  Phase 1: MVP 闭环基座 (1 个工作日)
  ├─ 引入 code.gitea.io/sdk/gitea 轻量客户端
  ├─ 实现 Task 行内 (est: ...) 与 Time Tracking 动态提取
  ├─ 改造 calendar.go：将真实工时注入飞书日历锁定
  └─ 落地单 Plan 的 5 小时滑动窗口平滑漏桶（消灭 429 报错）

  Phase 2: 完全体作曲家编排 (1 ~ 2 个工作日)
  ├─ 多 Plan / 多账号复调切调池（Claude + OpenAI + Gemini）
  ├─ 结合 workflow/graph.go 引擎推演 WBS 关键路径依赖
  ├─ 每周总预算大盘监控与自适应模型降调（Opus -> Sonnet -> Flash）
  └─ EWMA 方差校准与 Task 幂等防饥饿机制上线
```

---

## 七、 结论

通过本次梳理，我们将原本模糊的“限流防刷”理念，彻底升格为**“基于 Gitea Tasks 的工业级 WBS 路径编排体系”**。  
全篇规范已纳入 `key-agent` 内部档案，供后续团队架构评审与排期时作为权威蓝本。
