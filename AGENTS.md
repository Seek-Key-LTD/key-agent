# AGENTS.md — Key Agent 节点纪律

> 给 AI coding agent 与人类贡献者。**先读第 0–5 节**，那是本仓库自己的纪律。
> 第 6 节是上游 ADK-Go 的机制速查（fork 自 [google/adk-go](https://github.com/google/adk-go)，Apache 2.0），保留原文以便跟上游同步。

---

## 0. 这是什么

**多智能体系统里的"对抗"，通常在第一步就失败了：所有 agent 共享同一个运行环境，所以它们其实是一个人在自言自语。**

一个 agent 网络如果满足下面两条，它就没有对抗：

- 所有节点读同一份个人配置（`~/.claude`、`~/.codex`、`~/.config/...`）
- 所有节点跑同一套"驾驭工程"（harness）

**因为它们读的是同一套记忆、同一套习惯、同一套偏好——所以它们不是几个节点，是一个人的几个分身。**

本仓库做的事只有一件：

> **把"必须相同的"（核心）和"必须不同的"（外挂）在物理上分开。**
> **分开之后，两个节点之间除了协议层，没有任何共享状态——对抗才成立。**

Key Agent 是 Seek-Key 的 agent runtime，fork 自 google/adk-go。**只做加法**：ADK-Go 是 SDK 基座，一切业务能力（记忆、渠道、技能、身份、部署）都是可替换 adapter。

---

## 1. 为什么这样做 —— 加法的设计原理

**"只做加法"不是为了好维护。那只是工程理由。**

真正的原理是一句话：

| | 必须相同 | 必须不同 |
|---|---|---|
| **是什么** | **核心**：`runner/` `session/` `agent/` `model/` `tool/` | **外挂**：memory / channel / tool / LLM / 身份 / 分发 |
| **为什么** | 核心不同 → 两个节点说的东西**没法比** | 外挂相同 → 两个节点**没有差异**，等于同一个节点 |

合起来：

> **同核 + 异挂 = 可比的差异 = 对抗的最小条件。**

**"加法"就是实现这个分离的技术手段**——不是为了让上游好同步，是为了**让差异可复算**。

### 由此推出：为什么本仓库禁用 harness

harness（Claude Code / Codex / Antigravity / Cursor / 任何"套在 runtime 外面的一层"）**同时破坏两边**：

- **它是"核外的核"**——在 ADK-Go 外面又套一层，而那一层**每个人不同** → 核不同了 → 节点不可比。
- **它藏在 `~/.claude` 之类的地方**——不可见、不可复算 → 外挂变成黑箱 → 差异不可复算。

**所以"干掉 harness"不是洁癖，是加法纪律的直接推论。** 完整论证链：

```
ASN 要对抗
  → 对抗要可比的独立节点
    → 可比的独立节点 = 同核 + 异挂
      → 加法纪律
        → 干掉 harness（它同时破坏同核与异挂）
```

---

## 2. 怎么加 —— 四道合法门

**新能力只准走下面四道门。别的地方一律不许动。**

| 层 | 加什么 | 接口 | 例子 |
|---|---|---|---|
| **L1 Tool** | 一个函数 | `functiontool.New[Args, Results]` | 查一次 GBrain、算一笔账 |
| **L2 Toolset** | 一组工具 | 实现 `tool.Toolset` / `mcptoolset` | 接一个 MCP server |
| **L3 Plugin** | 横切行为 | `plugin.New(plugin.Config{...})` 的 `Before*`/`After*` | 审计、限额、脱敏 |
| **L4 Agent** | 新的 agent 类型 | `llmagent.New` / `agent.New` / `workflowagents/*` | 一个新的角色 |

**四道门之外的加法，就不是加法了**——那是改核心，属于第 4 节红线。

**每加一个外挂，必须登记：**

```
外挂名 / 类型（L1–L4）/ 依赖的后端 / model_id / dimension / version / 出网目标
```

**没有登记的外挂，不许进 main。**

---

## 3. 怎么隔离 —— 一个节点 = 一个实例 = 一个 `$HOME`

**这是本仓库最重要的一条。**

### 3.1 硬要求

1. **一个节点 = 一个实例**（进程 / 容器 / VM），**一个独立的 `$HOME`**
2. **不许两个节点共享 `$HOME`**
3. **不许挂载宿主的 `~`**（这是 harness 继承的唯一入口）
4. **镜像里不含任何 harness 配置**——没有可继承的东西
5. **二进制只从 OCA S3 / CF Worker 拉，SHA 校验**，不许自己 build 后分发
6. **出网白名单**：只准连 LiteLLM / GBrain / Authentik

### 3.2 为什么 `$HOME` 是那个关键点

"一人分饰多角"不是态度问题，**是环境问题**：

> **只要所有角色共享同一个 `$HOME`，它们就不可能真正独立——因为它们读的是同一套配置、同一套记忆、同一套习惯。**
> **共享 `$HOME` 的多个角色，其实是同一个人。**

### 3.3 这一条为什么非做不可

它直接决定"共时性"是真的还是假的：

| `$HOME` | 后果 |
|---|---|
| **共享** | 所有节点是**同一个观测站** → 共时性是假的 → **一个人的独白** |
| **独立** | 每个节点是**不同的观测站** → **共时性成立** |

**上下文隔离不是工程洁癖，是"多节点"这个概念的前提。**

---

## 4. 五道红线

**改这五个包 = 改 runtime = 破隔离。一律不许，除非走 ADR 且经 maintainer 批准。**

```
runner/    session/    agent/    model/    tool/
```

| # | 红线 | 为什么 |
|---|---|---|
| 1 | **不改核心五包** | 核一变，节点不可比 |
| 2 | **不引入 harness** | 见第 1 节，同时破坏同核与异挂 |
| 3 | **不共享 `$HOME`** | 见第 3 节 |
| 4 | **不绕过四道门加能力** | 绕过的能力不可登记、不可复算 |
| 5 | **不提交凭证 / 不混 embedding 索引** | 凭证进历史永久；不同模型向量不可混 |

> 注：第 5 条里的"不混 embedding 索引"来自 `docs/REQUIREMENTS.md` §5——**文本是本金，embedding 是可再生索引**，必须存 `model_id / dimension / version`，查询按 collection 的 embedding family 路由。

---

## 5. 怎么自检 —— 可执行的验收

**不用先理解这个项目的意义，跑这几条就知道自己有没有做错。**

### 5.1 隔离成立吗

```bash
# 两个节点的 $HOME 必须大量不同（期望：输出行数 > 0）
diff -rq /var/lib/key-agent/node-a/home /var/lib/key-agent/node-b/home | wc -l

# 且不应该出现任何 harness 残留（期望：无输出）
ls -a "$HOME" | grep -E '^\.(claude|codex|cursor|gemini|config/gh)$'
```

**如果 diff 是空的，或者有大量重叠——隔离没成立。**

### 5.2 只走了合法门吗

```bash
# 期望：✅ 无输出（没碰核心五包）
git diff --name-only origin/main...HEAD | grep -E '^(runner|session|agent|model|tool)/' \
  && echo "❌ 碰了核心，需要 ADR" \
  || echo "✅ 只走了外挂"
```

### 5.3 外挂登记了吗

```bash
# 期望：每个新增 adapter 都能在 registry 里找到
git diff --name-only origin/main...HEAD | grep -E '^(memory|session|messaging|evm|feishu)/' \
  | while read -r f; do grep -q "$f" docs/ADAPTER-REGISTRY.md || echo "❌ 未登记: $f"; done
```

### 5.4 版本统一吗

```bash
# 期望：本地二进制 SHA == 分发源 SHA
shasum -a 256 dist/k-agent-* | awk '{print $1}' | sort -u | wc -l   # 期望 1
```

### 5.5 交付前必跑（上游 ADK-Go 的定义）

```bash
test -f go.work || go work init
go work use -r .
go build -mod=readonly work
go test -race -mod=readonly -count=1 -shuffle=on work
golangci-lint run          # 每个 module 都要跑
go mod tidy -diff          # 必须无输出
```

---

## 6. 上游机制速查（ADK-Go 原文保留）

> 以下内容来自上游 `google/adk-go`，用于跟上游同步。**它讲的是"怎么给 ADK-Go 提 PR"，不是本仓库的节点纪律。**

### 6.1 项目概览

ADK Go（`google.golang.org/adk/v2`）是一个开源、code-first 的 Go 工具包，用于构建、评估、部署 AI agent。与 Gemini 优化集成，但模型无关。Go / Python / Java / Kotlin / TypeScript 多个实现共享概念模型但各自独立。需要 `go.mod` 声明的 Go 版本（当前 1.26.5）。

开发在 `main`（2.x 线）。`v1` 是 1.x 的维护分支。

### 6.2 安装与核心命令

本仓库是多模块：根模块 `google.golang.org/adk/v2` 加 `plugin/agentanalytics`。先建 workspace——`go.work` 是本地文件且被 gitignore，且 `go work init` 在已存在时会失败：

```bash
test -f go.work || go work init
go work use -r .
```

然后在仓库根执行（`work` 跨 workspace 内所有模块，`./...` 只匹配当前所在模块）：

- Build:       `go build -mod=readonly work`
- Test:        `go test -race -mod=readonly -count=1 -shuffle=on work`
- Single pkg:  `go test -race ./agent/...`
- Lint:        `golangci-lint run`（逐模块；CI 锁 v2.3.1，配置见 `.golangci.yml`）
- Tidy check:  `go mod tidy -diff`（逐模块；必须无输出）
- Format:      `golangci-lint fmt`（逐模块；按配置应用 gofumpt + goimports）

没有 `go.work` 时，`work` 会静默退化成只匹配根模块，且仍然退出 0——**所以绿色之前先确认 workspace 存在。**

### 6.3 完成定义（Definition of done）

1. `go build` 成功
2. `go test` 全绿
3. `golangci-lint run` 每个模块无发现
4. `go mod tidy -diff` 每个模块无输出
5. 新增/变更行为有测试；修 bug 要有能复现的测试
6. 每个新 Go 文件开头是 Apache 2.0 license header（由 `goheader` 强制）
7. 根 `go.mod` 不 require 仓库内子模块（由 `guardrail` CI job 强制）

### 6.4 仓库布局

- `agent/`     Agent 接口与类型（`llmagent`、`remoteagent`、`workflowagent`；`workflowagents/` 下有 `loopagent`、`parallelagent`、`sequentialagent`）
- `runner/`    驱动 run loop 的执行引擎
- `workflow/`  面向多 agent 应用的 node/graph 工作流引擎
- `model/`     LLM 抽象（`gemini`、`apigee`、`openaimodel`）
- `tool/`      Tool/Toolset 接口与内置工具（含 `skilltoolset/`、`mcptoolset/`）
- `session/`   会话状态与事件
- `memory/`、`artifact/`   长期记忆与文件/数据服务
- `auth/`      出站请求的凭证与认证 provider
- `agentregistry/`  Google Cloud Agent Registry 客户端（A2A agents、MCP servers、models）
- `plugin/`    横切生命周期钩子；`plugin/agentanalytics` 是独立模块
- `server/`    HTTP server（`adkrest` 为主；`adka2a`、`agentengine`）
- `cmd/`       CLI（`adkgo`）与 server launcher
- `telemetry/`、`util/`   公共辅助包
- `platform/`  时间与 UUID 生成的可覆盖 seam（确定性测试用）
- `internal/`  私有包——非公共 API；`internal/httprr` 是 vendored
- `examples/`  可运行示例 agent
- `scripts/`   仓库工具

### 6.5 约定与惯用法

- **流式**：agent run 返回 `iter.Seq2[*session.Event, error]`，用 `for event, err := range … {}` 消费。**不要收集成 slice。**
- **接口优先**：公共包暴露接口（`Agent`、`Tool`、`Toolset`、`Service`）；具体实现放子包或 `internal/`。
- **回调优先于继承**（Agent/Model/Tool 的 `Before*`/`After*`）；`Before` 回调返回非 nil 会短路执行。
- **错误**：用 `fmt.Errorf("…: %w", err)` 包装。工具确认用 sentinel error（如 `tool.ErrConfirmationRequired`）。
- 优先复用已有 helper；包保持小而专。

### 6.6 最小示例

```go
model, err := gemini.NewModel(ctx, "gemini-2.5-flash",
    &genai.ClientConfig{APIKey: os.Getenv("GOOGLE_API_KEY")})
// handle err
a, err := llmagent.New(llmagent.Config{
    Name:        "assistant",
    Model:       model,
    Instruction: "You are a helpful assistant.",
    Tools:       []tool.Tool{ /* ... */ },
})
// handle err
r, err := runner.New(runner.Config{
    AppName:           "my-app",
    Agent:             a,
    SessionService:    session.InMemoryService(),
    AutoCreateSession: true,
})
// handle err
msg := genai.NewContentFromText("Hello", genai.RoleUser)
for event, err := range r.Run(ctx, userID, sessionID, msg, agent.RunConfig{}) {
    // handle err; read event.LLMResponse.Content
}
```

完整可运行程序见 `examples/quickstart`。

### 6.7 测试

- 测试**默认离线**：LLM HTTP 流量通过 `internal/httprr` 从 `testdata/*.httprr` 重放。**永远不要在测试里加真实模型或网络调用。**
- 要（重）录制某个包的流量：提供真实凭证（如 `GOOGLE_API_KEY`）并跑 `go generate ./<pkg>/...`，然后提交更新后的 `testdata/*.httprr`。
- 优先表驱动测试；共享 helper 在 `internal/testutil`。

### 6.8 上游边界

**Always**：交付前跑 build/test/lint/tidy；PR 小且单一关注点；为你改的代码加/更新测试。

**Ask first**：加或升依赖；改高扇入包；任何公共 API 变更与破坏性变更。

**Never**：破坏公共 API；编辑 vendored 代码（`internal/httprr`）或提交密钥；加会发真实 LLM/网络调用的测试。

### 6.9 与 adk-python 对齐

[adk-python](https://github.com/google/adk-python) 是功能行为的 source of truth。移植或验证功能时，对照 Python 实现检查一致性。

### 6.10 资源

- Docs: https://google.github.io/adk-docs/
- Examples: `./examples`
- 其他 ADK 实现：[Python](https://github.com/google/adk-python) · [Java](https://github.com/google/adk-java) · [Kotlin](https://github.com/google/adk-kotlin) · [TypeScript](https://github.com/google/adk-js)

---

## 附：与 `docs/REQUIREMENTS.md` 的关系

`docs/REQUIREMENTS.md` §5 的六条纪律（只做加法 / 文本是本金 / 不混索引 / GBrain 只收精选 / 不改上游公共 API / 不绑定云厂商）是本文件的**技术细则**。

**本文件管的是"为什么"和"红线"；`REQUIREMENTS.md` 管的是"当前要做什么"。** 两者冲突时，以本文件的设计原理为准。
