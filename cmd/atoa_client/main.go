// A2A 客户端测试：通过 Consul delegations 发现远程 agent 并调用。
// 用法: AGENT_NAME=ruby AUTHENTIK_CLIENT_ID=agent-client-ruby AUTHENTIK_CLIENT_SECRET=... CONSUL_ADDR=http://127.0.0.1:8500 \
//       go run ./cmd/atoa_client topaz "任务文本"
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/adk/v2/internal/atoa"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: atoa_client <to-agent> <task-text>")
		os.Exit(1)
	}
	toAgent := os.Args[1]
	taskText := os.Args[2]

	client, err := atoa.NewClientFromEnv()
	if err != nil {
		fmt.Println("client init error:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Printf("[%s → %s] %s\n", client.From, toAgent, taskText)
	reply, err := client.SendMessage(ctx, toAgent, taskText)
	if err != nil {
		fmt.Println("A2A call error:", err)
		os.Exit(1)
	}
	fmt.Printf("✅ %s 回复: %s\n", toAgent, reply)
}
