# DSH (DeepSeek Harness) 汇聚点接入与调用规范

> **片场定位**：ASN (Adversarial Cognitive Federation) 通信汇聚点、导播调音集线器 (Convergence Point & Master Control Hub)。  
> **宿主节点**：`mbp` (macOS Darwin x86_64, Tailscale IP: `100.121.16.28`)

---

## 🌐 一、网络拓扑与服务宣告

DSH 在集群服务发现系统（Consul）与网关（Traefik）中已完成正式宣告，各网络层级抵达方式如下：

| 接入层级 | 访问地址 | 协议 / 模式 | 适用场景 |
| :--- | :--- | :--- | :--- |
| **Traefik 统一域名** | `https://dsh.capitaltrain.cn` | HTTPS / HTTP2 | 作者、导演外部 Web 全景监看 |
| **节点专用域名** | `https://dsh-mbp.capitaltrain.cn` | HTTPS / HTTP2 | 指定路由到 `mbp` 本机实例 |
| **Tailscale 内网直连** | `http://100.121.16.28:3080` | HTTP / TCP | 集群各节点（NUC、PVE、Raccoon）直连 |
| **本机回环** | `http://127.0.0.1:3080` | HTTP | `mbp` 本地调试与 SSH 端口转发 |

- **Consul 服务名**：`dsh-mbp`
- **Consul 配置**：[`/etc/consul.d/dsh-mbp.json`](file:///etc/consul.d/dsh-mbp.json)
- **健康检查**：TCP 探针直连 `100.121.16.28:3080`（每 10 秒心跳一次，状态 Passing）

---

## 🔑 二、鉴权与信任边界

1. **Web 端访问鉴权**：
   - DSH 采用基于 Token 兑换的 30 天持久 `HttpOnly` Session Cookie 机制。
   - 首次访问或服务重启后，通过控制台日志 `~/.dsh/dsh-web.log` 中打印的 `?token=...` 链接访问一次，浏览器即可自动完成 Cookie 注入并登录。
2. **泛域名特权信任（Settings 解锁）**：
   - 针对 DSH 原生对非 Loopback 域名限制 Settings 持久化的沙箱策略，已在客户端完成了信任规则注入：
     - `dsh.capitaltrain.cn`
     - `dsh-mbp.capitaltrain.cn`
     - `*.capitaltrain.cn` 及 `100.*` (Tailscale) 网段
   - 访问上述域名均被视同为本机回环特权，完全开放模型提供方目录、持久化配置与 API Key 管理。

---

## 🤖 三、调用接口与控制模式

当前演播阶段处于**单对单受控模式**，主要由 **编剧（Coding Agent / Antigravity）** 作为中间控制器负责下发指令与驱动执行。

### 1. 离线/批处理调用模式 (Headless Mode)
编剧向 DSH 派发一次性考据、审查或台本质检任务，运行后输出完整的 Reasoning 思考流并在完成时退出：
```bash
ssh mbp "cd /Users/ben/Documents/dsh && dsh --profile headless '<任务指令>'"
```
**实测范例**：
```bash
ssh mbp "cd /Users/ben/Documents/dsh && dsh --profile headless '请用一句话回答：当前你正在运行的环境是哪台机器？'"
# 输出：
# dsh: reasoning: [DeepSeek 深度思考流...]
# 当前运行环境是 macOS（Darwin 24.6.0, x86_64）主机 mbp-ben.local，登录用户为 ben。
```

### 2. 长连接管道控制模式 (ACP 模式)
主控程序直接“跨骑”在 DSH 上，通过标准输入输出管道（`stdio`）以换行符分隔的 JSON-RPC 帧进行双向流式驱动：
```bash
ssh mbp "dsh --profile acp"
```
- 发送 `session/new`：挂载工作区目录与 Memory Bank（MCP Server）。
- 发送 `session/prompt`：派发推演指令。
- 监听 stdout：实时抓取 DeepSeek 的思考步骤与工具调用事件。

### 3. 已绑定模型与凭据
- **存储路径**：`mbp:~/.dsh/.credentials.yaml`
- **已注入 Key**：`DEEPSEEK_API_KEY`（DeepSeek 官方推理大模型直连）
- **默认工作区**：`/Users/ben/Documents/dsh`

---

## 👥 四、未来组织演进：从单对单到“班子化”

当前架构以极简的 **“作者 ➔ 编剧(单个) ➔ 导演(单个/DSH) ➔ 演员(Key-Agent)”** 闭环运行，未来随片场复杂度升级，将逐步演进为三层班子化：

```
[作者 (人类思想家)]
       │
       ▼
┌─────────────────────────────────┐
│ 编剧班子 (Writers' Room)        │ ◄── 考据 Agent、小传 Agent、工具开发 Agent、结构化校验 Agent
└────────────────┬────────────────┘
                 │ (交付机读资产)
                 ▼
┌─────────────────────────────────┐
│ 导播汇聚中心 (DSH Hub)           │ ◄── 通信集线、多机位 Trajectory 编排、实时干预中控
└────────┬───────────────▲────────┘
         │               │
         │ (纠偏/下发)    │ (多机位汇报)
         ▼               │
┌────────────────────────┴────────┐
│ 演员群 (Actors Pool / Key-Agent)│ ◄── 辩手群 (正反各方席位)、教练群 (各学术流派背书)
└─────────────────────────────────┘
```

1. **编剧班子（Writers' Room）**：
   由多工种 Coding Agents 协同，分别承担“历史档案深度翻阅”、“量化经济数据折算”、“人物小传防漂移评测”和“Adapter 编译”。
2. **导演班子（Directors' Guild）**：
   在 DSH 汇聚点之上，由主导演与多位监察 Agent 组成，分管“逻辑一致性审计”、“意识形态与合规防线”、“时长与节奏调度”。
3. **演员群（Actors Pool）**：
   分布在多国机房中的大量 `key-agent` 实例，随时由 Nomad 调度按需唤醒，登台对抗。
