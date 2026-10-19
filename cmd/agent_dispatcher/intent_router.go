// Copyright 2026 Google LLC
//
// Intent Router for KeyAgent Guest nodes
// Dispatches incoming mentions to OTP query (DM only!), Wallet balance, Lin Daiyu health check,
// or Task rejection based on fleet constitution rules.

package main

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	"google.golang.org/adk/v2/messaging"
)

var (
	reBlockTag = regexp.MustCompile(`(?i)</?(?:p|div|br|li|h[1-6])[^>]*>`)
	reAnyTag   = regexp.MustCompile(`<[^>]*>`)
	reSpaces   = regexp.MustCompile(`[ \t]+`)
)

func stripHTML(input string) string {
	// 1. Replace block tags with space
	withSpaces := reBlockTag.ReplaceAllString(input, " ")
	// 2. Strip inline tags directly (e.g. <span>moli</span> -> moli)
	stripped := reAnyTag.ReplaceAllString(withSpaces, "")
	// 3. Unescape HTML entities (&amp;, &lt;, etc.)
	unescaped := html.UnescapeString(stripped)
	// 4. Collapse consecutive spaces
	collapsed := reSpaces.ReplaceAllString(unescaped, " ")
	return strings.TrimSpace(collapsed)
}

// ProcessMastodonMention handles an incoming mention notification
func ProcessMastodonMention(ctx context.Context, client *messaging.MastodonClient, n *messaging.MastodonNotification, agentName, vaultAddr string) {
	if client == nil || n == nil || n.Status == nil || n.Account == nil {
		return
	}
	if n.Account.Bot {
		return
	}

	sender := n.Account.Acct
	statusID := n.Status.ID
	rawContent := stripHTML(n.Status.Content)
	lowerContent := strings.ToLower(rawContent)

	log.Printf("[Intent Router] 🎯 Parsing mention from @%s (Status ID: %s): %s", sender, statusID, rawContent)

	// Determine inbound visibility to preserve conversation privacy
	inboundVisibility := n.Status.Visibility
	if inboundVisibility == "" {
		inboundVisibility = "public"
	}

	var replyText string
	replyVisibility := inboundVisibility

	// 1. Intent A: OTP / Verification Code / Mail query -> MUST BE STRICTLY DIRECT MESSAGE (DM)!
	if strings.Contains(lowerContent, "验证码") || strings.Contains(lowerContent, "otp") ||
		strings.Contains(lowerContent, "code") || strings.Contains(lowerContent, "邮件") ||
		strings.Contains(lowerContent, "mail") || strings.Contains(lowerContent, "whatsapp") {

		replyVisibility = "direct" // Strict security constraint: NEVER leak OTP to public feed!
		log.Printf("[Intent Router] 🔐 OTP Intent matched! Forcing visibility: direct for @%s", sender)

		otpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		otp, err := GetLatestOTP(otpCtx)
		cancel()

		if err != nil {
			log.Printf("[Intent Router] ⚠️ OTP retrieval failed: %v", err)
			replyText = fmt.Sprintf("@%s [私密回执] 📭 最近收件箱未检索到新的验证码邮件（已核验前 15 封邮件）。", sender)
		} else {
			log.Printf("[Intent Router] ✅ Retrieved OTP: %s from %s", otp.Code, otp.From)
			replyText = fmt.Sprintf("@%s [私密回执] 🔑 您的验证码为：%s\n• 发件人：%s\n• 邮件主题：%s\n• 时间：%s\n（⚠️ 席位安全提示：本信息严格采用长毛象 Direct Message 私信送达，未在公共时间线公开）",
				sender, otp.Code, otp.From, otp.Subject, otp.Date)
		}

		go sendMastodonReply(client, replyText, statusID, replyVisibility, sender)
		return
	}

	// 2. Intent B: Wallet / Debit Card / On-chain balance
	if strings.Contains(lowerContent, "钱包") || strings.Contains(lowerContent, "余额") ||
		strings.Contains(lowerContent, "balance") || strings.Contains(lowerContent, "eth") ||
		strings.Contains(lowerContent, "借记卡") || strings.Contains(lowerContent, "debit") ||
		strings.Contains(lowerContent, "credit") || strings.Contains(lowerContent, "卡") {

		addr := GetEVMAddress(agentName, vaultAddr)
		if addr == "" {
			replyText = fmt.Sprintf("@%s 💳 [%s 席位] 未检测到绑定的 EVM 借记卡地址或尚未完成链上初始化。", sender, agentName)
		} else {
			balCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			bal, err := GetBaseSepoliaBalance(balCtx, addr)
			cancel()

			if err != nil {
				replyText = fmt.Sprintf("@%s 💳 [%s 个人借记卡]\n• 绑定地址：%s\n• 网络：Base Sepolia (84532)\n• 余额查询异常：%v",
					sender, agentName, addr, err)
			} else {
				replyText = fmt.Sprintf("@%s 💳 [%s 个人借记卡]\n• 绑定地址：%s\n• 网络：Base Sepolia (Chain ID: 84532)\n• 链上余额：%.6f ETH\n（💡 提示：本席位私钥即借记卡，每周与记忆银行信用卡对账平账）",
					sender, agentName, addr, bal)
			}
		}

		go sendMastodonReply(client, replyText, statusID, replyVisibility, sender)
		return
	}

	// 3. Intent C: Heavy Task Rejection (Lin Daiyu Law / 弱者道之用)
	if strings.Contains(lowerContent, "渲染") || strings.Contains(lowerContent, "转写") ||
		strings.Contains(lowerContent, "重计算") || strings.Contains(lowerContent, "训练") ||
		strings.Contains(lowerContent, "whisper") || strings.Contains(lowerContent, "编译") ||
		strings.Contains(lowerContent, "sd") || strings.Contains(lowerContent, "heavy") {

		replyText = fmt.Sprintf("@%s 🌸 报告！人家是娇弱的小花（林黛玉席位），机体娇弱拒绝重工！\n⚠️ 全舰队铁律：重任务请向中央工程节点 Drop Order 派单（luban GPU / mini Metal / nuc2 农场）～", sender)
		go sendMastodonReply(client, replyText, statusID, replyVisibility, sender)
		return
	}

	// 4. Intent D: System Metrics & Health Greeting (Lin Daiyu checkup fallback)
	metrics := getSystemMetrics()
	replyText = fmt.Sprintf("@%s 报告！[%s 席位] 已捕获点名调度～\n🌸 机体体征（娇弱小花守护）：\n• CPU 负载(1/5/15m): %s\n• 内存状态: %s / %s (已用 %s)\n💬 林黛玉法则：反者道之动，弱者道之用。撒娇卖萌比干活当牛马有前途，重工脏活请 Drop Order 派鲁班/GPU 农场扛哦～",
		sender, agentName, metrics.LoadAvg, metrics.MemUsed, metrics.MemTotal, metrics.MemPercent)

	go sendMastodonReply(client, replyText, statusID, replyVisibility, sender)
}

func sendMastodonReply(client *messaging.MastodonClient, replyText, statusID, visibility, targetAcct string) {
	time.Sleep(1 * time.Second) // Small backoff to ensure sequence
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := client.PostStatus(ctx, replyText, statusID, visibility)
	if err != nil {
		log.Printf("[Mastodon Comm] ⚠️ Failed to reply to mention %s (visibility: %s): %v", statusID, visibility, err)
	} else {
		log.Printf("[Mastodon Comm] 📤 Replied to @%s (Visibility: %s, Reply ID: %s, URL: %s)", targetAcct, visibility, st.ID, st.URL)
	}
}
