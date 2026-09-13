# Key Agent 社区贡献与审核员守则

欢迎加入 **Key Agent**（面向因陀罗网络 ASN 与去中心化认知联合体的高性能单进程 Agent 运行时）！

---

## 核心开发与评审原则

1. **单进程高内聚（Single-Process High Cohesion）**：
   - 保持运行时轻量、确定性与跨跨 DC 穿透能力；
   - 尽量减少不必要的外部重型 SDK 侵入，优先使用纯 Go 实现。
2. **零凭据暴露与零付费账单（Zero Secrets & Zero Billing）**：
   - 任何涉及 Vault、Authentik、EVM 私钥的测试必须使用 Mock 或环境变量注入，严禁硬编码；
   - 社区协作遵循自带干粮原则，不消耗组织账户的付费额度。
3. **肯定性立论与测试覆盖（Test Driven & Affirmative）**：
   - 提交新特性或修复时，必须附带完整的 `_test.go` 单元测试并通过 CI 验证。

---

## 审核员（Reviewer）核验清单

- [ ] **Go 编译与测试**：`go test ./...` 必须全部通过，无竞态条件警告（`-race`）。
- [ ] **安全审计**：无硬编码的 IP、私钥、Token 原文。
- [ ] **IaC 声明规范**：Docker / Nomad / Authentik 模板遵循最小权限原则。
