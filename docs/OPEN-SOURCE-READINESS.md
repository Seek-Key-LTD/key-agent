# 开源就绪评估 —— 改名、模块路径与推送阻塞

> **日期**：2026-09-13
> **结论摘要**：仓库改名已完成；推送 GitHub 被 **Push Protection** 拦截
> （历史含 Vault 令牌）。主理人评估为网络隔离下可接受，待解除拦截。

---

## 一、改名：已完成

| 项 | 状态 |
|---|---|
| GitHub 仓库 | `Seek-Key-LTD/adk-go` → **`Seek-Key-LTD/key-agent`** ✅ |
| fork 关系 | 保留（仍是 `google/adk-go` 的 fork，公开） |
| Go 模块路径 | **未改**，仍为 `google.golang.org/adk/v2` |

### 1.1 为什么保留模块路径

**仓库名与 Go 模块路径是两件独立的事**，改仓库名不影响 `go.mod`，不会破坏编译。

改模块路径的代价对比：

| 方案 | 好处 | 代价 |
|---|---|---|
| **保持路径**（现方案） | 上游可 `git merge` 合并 | 不能被 `go get`，只能以二进制分发 |
| 改模块路径 | 成为独立 Go 模块 | **上游合并变成噩梦**——1660 处 import 全部不同，处处冲突 |

README 已声明「**上游同步保持兼容**」，说明上游合并是要保的能力。
且本仓已在做 5 架构 CI 编译与二进制分发，不依赖 `go get`。
**故保留模块路径。**

### 1.2 fork 关系的两个约束

1. 保留 fork 关系 → GitHub 会一直显示 "forked from google/adk-go"（**这是诚实的**）
2. **fork 在父仓公开时不能设为 private**。将来若要私有，须先脱离 fork 关系
   （新建仓库推历史，或联系 GitHub Support）

### 1.3 改造量实测

| 项 | 数量 |
|---|---|
| 我方触及文件 | 53 |
| 其中 `.go` | 30 —— **全部为新增，零个上游 `.go` 被修改** |
| 被改的上游文件 | 4：`go.mod` `go.sum` `README.md` `.gitignore` |

**≈96% 是纯加法**，是个很干净的 fork。

---

## 二、⚠️ 推送阻塞：历史含存活 Vault 令牌

### 2.1 发现

推送前扫描（范围：`5337567..HEAD`，67 个提交）发现：

```
cmd/test_oracle_lake5_memory/test_oracle_26ai.py:13
VAULT_TOKEN = "hvs.<已打码>"        # 前缀 hvs. = HashiCorp Vault 服务令牌
```

> ⚠️ **本文档此前的版本把令牌原文抄了进来**，等于自己造了第二个泄露点
> （GitHub 推送保护一并报出）。**写审计文档时只写前缀，不写原文。**

- `hvs.` = HashiCorp Vault 服务令牌前缀
- 该令牌读取 `secret/data/oracle/config/lake5`，**其中存放 Oracle ADB 的账号与密码**
- 同文件还硬编码了 Vault 内网地址与 HAProxy 内网地址
- **已确认该令牌尚未出现在 GitHub 上** —— 若不扫描直接推送，即为**新泄露**

### 2.2 已做

1. `test_oracle_26ai.py`：`VAULT_ADDR` / `VAULT_TOKEN` / `VAULT_PATH` /
   `HAPROXY_HOST` / `HAPROXY_PORT` 全部改为**环境变量注入**，
   缺失即 fail loudly，**不回退到硬编码值**
2. `.gitea/workflows/doc-gate.yml` 密钥检查从 1 条扩到 **3 条**：
   - 通用密钥赋值（原有）
   - **已知令牌前缀**：Vault `hvs.`/`hvb.`/`hvr.` · GitHub `ghp_`/`gho_` ·
     Notion `ntn_` · OpenAI `sk-` · Gemini `AIza` · Cloudflare `HYT-`
   - **内网地址**：RFC1918（`192.168.` / `10.` / `172.16-31.`）
     \+ Tailscale CGNAT `100.64/10`

   两条新规则已本地自测命中。

### 2.3 推送结果：GitHub 推送保护拦截

实测推送被 GitHub **Push Protection** 拒绝：

```
remote: error: GH013: Repository rule violations found for refs/heads/main.
remote: - GITHUB PUSH PROTECTION
remote:   —— HashiCorp Vault Root Service Token ——
remote:    locations:
remote:      - commit: 1e879cf…  cmd/test_oracle_lake5_memory/test_oracle_26ai.py:13
remote:      - commit: 8b1bea2…  docs/OPEN-SOURCE-READINESS.md:57
```

> **第二处是本文档自己造成的**：本文此前把令牌原文抄了进来。
> **已修正。教训：写审计文档时只写前缀，不写原文。**

GitHub 将其识别为 **Root Service Token**（比普通 service token 更高权限）。

### 2.4 主理人判断：网络隔离下可接受

主理人评估：**该 Vault 在 Tailscale 网络内，外部知道也无法连接**，故无需吊销。

**实测证据（支持该判断）**：

| 检查 | 结果 |
|---|---|
| 本机 tailnet 状态 | 在网（`100.86.9.29 mini`） |
| `authentik/gitea/matrix/litellm.capitaltrain.cn` 解析 | **全部 → `100.93.5.81`**（同一个 Tailscale IP） |
| 该地址所属网段 | **`100.64.0.0/10`（RFC 6598 CGNAT）——公网不可路由** |
| 67 个提交中的**云侧凭据**（GitHub/Cloudflare/Notion/OpenAI/Gemini 前缀） | **零** |

> **关键区分**：网络隔离保护**内网服务**，**不保护云侧凭据**——
> 后者从任何地方都能用。本次扫描确认待推内容里没有云侧凭据，
> 所以隔离判断成立。

### 2.5 两条路

| 方案 | 做法 | 说明 |
|---|---|---|
| **A. 解除拦截**（符合主理人判断） | 点 GitHub 报错里给的 unblock 链接 | 一次性放行，记录在安全日志；令牌留在历史里 |
| **B. 改史清除** | `git filter-repo` 清掉该文件的历史 | force-push；历史干净，但协作方需重 clone |

> 若将来该 Vault 出现**任何公网暴露**（新 ingress / 端口转发 / 临时调试暴露），
> 已公开的令牌即刻可用。这是**条件性安全**，不是错误——
> 但要意识到条件是什么。

---

## 三、其他发现（未改，需部署侧配合）

### 3.1 残留硬编码内网地址

| 文件 | 内容 |
|---|---|
| `cmd/agent_dispatcher/main.go:32` | `vaultAddr` 默认值 = Vault 内网地址 |
| `integration/dispatcher.go:30` | `HAProxyHost` = 内网地址 |
| `integration/dispatcher.go:31` | `ServiceName` = **真实 Oracle ADB 服务名** |

**未改的原因**：这两处是**运行中的 dispatcher 服务**的默认配置，
改为环境变量注入会**改变部署行为**，需配合 rollout。

`integration/dispatcher.go` 里的 `Password: "placeholder_pass"` 与
`AccessToken: "syt_agent_bot_token"` **是占位符，不是真实凭据**，无需处理。

### 3.2 仓库卫生

- `dist/oracle-agent-linux-amd64` —— **36MB 编译产物被提交进仓库**，
  建议移出并加入 `.gitignore`
- 本地领先 GitHub **67 个提交**（5.5 周）

---

## 四、待办清单

- [ ] **P0** 决定推送路径：**A. 点 unblock 链接放行**（符合主理人网络隔离判断）
      或 **B. 改史清除**。见 §2.5
- [ ] P1 把 `cmd/agent_dispatcher` 与 `integration/dispatcher.go` 的内网地址
      迁到环境变量（需部署配合，会改变运行中服务的行为）
- [ ] P2 移出 `dist/` 下的二进制产物（36MB）
- [ ] P2 确认 `dist/`、`*.log`、`.env*` 已在 `.gitignore`
- [ ] P3 建议开启 GitHub **Secret Scanning**（该仓已具备资格）

---

## 五、复现命令

```bash
# 推送前扫描（范围：GitHub 已有 HEAD..本地 HEAD）
GH=$(gh api repos/Seek-Key-LTD/key-agent/commits/main --jq '.sha')
git diff "$GH"..HEAD > /tmp/topush.diff

# 令牌前缀
grep -nE 'hvs\.|hvb\.|hvr\.|ghp_[A-Za-z0-9]|gho_[A-Za-z0-9]|ntn_[A-Za-z0-9]|sk-[A-Za-z0-9]{16,}|AIza[A-Za-z0-9_-]{30,}|HYT-' /tmp/topush.diff

# 内网地址
grep -nE '192\.168\.[0-9]+\.[0-9]+|10\.[0-9]+\.[0-9]+\.[0-9]+|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.[0-9]+\.[0-9]+' /tmp/topush.diff

# 该令牌是否已在 GitHub 上
git grep -nE 'hvs\.[A-Za-z0-9]{16,}' "$GH"
```
