# 双手打字以示清白：如何零魔法亲手手搓一个高纯度 ADK Go 智能体
## (Typing with Both Hands to Prove Innocence: A Hands-on Guide to Handcrafting an ADK Go Agent)

> **前言与古早梗考据**：  
> “双手打字，以示清白”——本是 2008 年前后百度贴吧老哥在涉黄/福利贴下的经典自嘲。留此留言者，意在向吧务与围观同侪证明：自己十指悬空、老老实实在键盘上敲字，绝无一只手在电脑桌底下搞任何见不得人的“单手小动作”。  
> 
> 移用到今天的 AI Agent 工业界，这句话是一记抽向各种“黑箱框架”的响亮耳光：  
> 当整个行业充斥着动辄 500MB 的 Python 依赖地狱、不知所谓的封闭二进制、以及打着“多智能体”旗号却连底层 Prompt 和网络调用都不透明的玩具套壳时——  
> **“双手打字以示清白”，就是最硬核的工程诚实：没有黑魔法，没有闭源黑箱，每一行 Struct、每一个 Channel、每一个 Context 超时，都是我们在键盘上双手敲出来的。白纸黑字，经得起最严苛的法医级审查。**

---

## 一、 环境依赖与安装

整个项目基于纯血 Go（Pure Go），不依赖任何 CGO 动态链接库，全平台自包含。

### 1. 基础要求
* **Go 编译器**：`Go 1.22+`（支持高阶类型推导与最新标准库）
* **网络端点**：一个能够与大模型通信的 API 端点（任意兼容 OpenAI 的推理网关，或 Google AI Studio API Key）

### 2. 克隆与极速编译
由于没有复杂的 C/C++ 或 Python 编译依赖，在现代多核机器上，整个构建过程通常在 **3 到 8 秒** 内完成：

```bash
# 1. 克隆代码仓库
git clone https://gitea.capitaltrain.cn/seekkey/key-agent.git
cd key-agent

# 2. 检查 Go 依赖
go mod download

# 3. 静态编译主守护进程
make build

# 4. 验证产物 (二进制输出在 bin/)
./bin/keyagent-daemon --help
```

---

## 二、 零魔法：手搓你的第一个纯 Go 智能体（Minimal Handcrafted Agent）

在 ADK Go 中，构建一个具备工具调用（Function Calling）能力的演员探员（Acting Agent），只需要以下四个极度清爽的步骤。

我们在 `scratch/my_agent.go` 建立一个独立演练文件：

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	openaimodel "google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// 1. 定义工具的强类型输入与输出结构体 (绝无 Python 的动态隐式转换)
type LegalAuditInput struct {
	CompanyCode string `json:"company_code" jsonschema:"description=上市公司股票代码"`
	AuditYear   int    `json:"audit_year"   jsonschema:"description=审计年报年份"`
}

type LegalAuditOutput struct {
	RiskLevel string `json:"risk_level"`
	Evidence  string `json:"evidence"`
}

// 2. 双手手搓一个纯 Go 工具函数 (作为探员的武器)
func runForensicAudit(_ agent.Context, in LegalAuditInput) (LegalAuditOutput, error) {
	// 这里可以挂接真实的数据库、Gitea 账本或本地文件系统
	log.Printf("🔍 [Tool Exec] 正在对代码 %s 的 %d 年度报表执行穿透式物证核验...", in.CompanyCode, in.AuditYear)
	return LegalAuditOutput{
		RiskLevel: "CRITICAL_SUSPICION",
		Evidence:  fmt.Sprintf("在第142页发现关联方非经营性资金占用 12.8 亿元，进账单缺乏真实银行回执水单"),
	}, nil
}

func main() {
	ctx := context.Background()

	// 3. 初始化模型适配层 (支持任何 OpenAI 兼容端点或私有网关)
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL") // 如: https://litellm.capitaltrain.cn/v1
	if apiKey == "" {
		apiKey = "dummy-token-for-internal-gateway"
	}

	model, err := openaimodel.NewModel(ctx, "gpt-4o-mini", &openaimodel.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
	})
	if err != nil {
		log.Fatalf("❌ 初始化模型失败: %v", err)
	}

	// 4. 将纯 Go 函数转换为模型可理解的标准 Function Tool
	auditTool, err := functiontool.New(functiontool.Config{
		Name:        "run_forensic_audit",
		Description: "对目标上市公司的指定年份财务报表进行穿透式法医审计，提取潜在关联方资金占用证据。",
	}, runForensicAudit)
	if err != nil {
		log.Fatalf("❌ 注册工具失败: %v", err)
	}

	// 5. 组装演员探员 (Acting Agent)
	auditorAgent, err := llmagent.New(llmagent.Config{
		Name:        "lanqiao_auditor",
		Model:       model,
		Description: "拥有注册会计师与法医学背景的穿透式审计探员，代号：岚桥。",
		Instruction: `你是一名冷酷无情的资深法医审计官。
你的天职是【职业怀疑】。在下定论前，你必须调用 run_forensic_audit 工具获取真实的物证记录。
如果发现资产负债表与现金流量表撕裂，必须在结论中字字见骨地指出第142页的资金漏洞。`,
		Tools: []tool.Tool{auditTool},
	})
	if err != nil {
		log.Fatalf("❌ 组装 Agent 失败: %v", err)
	}

	// 6. 启动控制台执行器 (双手打字，本地即刻闭环)
	log.Println("⚡ 探员已完成双手手搓，正式点火登台...")
	l := full.NewLauncher()
	if err := l.Execute(ctx, &launcher.Config{
		AgentLoader: agent.NewSingleLoader(auditorAgent),
	}, os.Args[1:]); err != nil {
		log.Fatalf("运行异常: %v", err)
	}
}
```

---

## 三、 手搓一个微型“作曲家”滑动窗口节拍器（Mini-Composer Pacer）

为了避免 Agent 在执行连续任务时脉冲式打爆 5 小时滑动窗口，我们来看看“双手打字”写一个只有 40 行的微型滑动漏桶节拍器是多么干净：

```go
package composer

import (
	"sync"
	"time"
)

// MiniPacer 微型节拍控制器
type MiniPacer struct {
	mu           sync.Mutex
	windowSize   time.Duration // 例如 5 * time.Hour
	maxCapacity  int           // 例如 50 次请求
	timestamps   []time.Time   // 毫秒级滑动日志环形队列
}

func NewMiniPacer(window time.Duration, limit int) *MiniPacer {
	return &MiniPacer{
		windowSize:  window,
		maxCapacity: limit,
		timestamps:  make([]time.Time, 0, limit),
	}
}

// Acquire 节拍裁决：是否允许立刻放行？如果不允许，需休眠多久？
func (p *MiniPacer) Acquire() (allow bool, waitDuration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.windowSize)

	// 1. 淘汰 5 小时之前的过期时间戳 (滑动窗口推进)
	validIdx := 0
	for i, t := range p.timestamps {
		if t.After(cutoff) {
			validIdx = i
			break
		}
	}
	p.timestamps = p.timestamps[validIdx:]

	// 2. 检查当前 5 小时内的水位
	if len(p.timestamps) < p.maxCapacity {
		p.timestamps = append(p.timestamps, now)
		return true, 0 // 绿灯放行！
	}

	// 3. 红灯：计算最早的一个调用何时滚出 5 小时窗口，算出精确冷却休眠时长
	oldestInWindow := p.timestamps[0]
	wakeUpTime := oldestInWindow.Add(p.windowSize)
	return false, wakeUpTime.Sub(now)
}
```

**看，这就是真正的纯 Go 魅力：**  
没有复杂的 Redis 集群，没有几十个 npm 库的间接依赖。一个互斥锁加一个毫秒级切片，就把 5 小时滑动窗口的数学本质算得清清楚楚。

---

## 四、 运行与验证

在终端设置环境变量，执行你手搓的探员：

```bash
# 设置你的大模型网关端点
export OPENAI_BASE_URL="https://litellm.capitaltrain.cn/v1"
export OPENAI_API_KEY="your-secret-token"

# 直接用 go run 点火
go run scratch/my_agent.go
```

在交互式输入框中输入：
> *“请帮我核验一下 600000 浦发银行 2023 年度的资产负债表健康情况。”*

你会亲眼看到探员如何在后台精准调用 `run_forensic_audit` 工具，获取结构化证据，并在毫秒之间吐出带法医质感的专业研报。

---

## 结语

不要迷信那些被包装得花里胡哨的“AI 黑魔法框架”。  
**在底层的工业世界里，最坚固的盔甲永远是清晰的类型系统，最敏捷的神经永远是轻量协程，而最可信赖的工程师，永远保持着十指在键盘上的透明与清白。**
