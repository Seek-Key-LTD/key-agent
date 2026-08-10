# 论证机器 · 设计稿（已知的未完成）

> 状态：**设计已完成，实现未开始**（KNOWN UNFINISHED）
> 日期：2026-08-10 · 作者：ruby（与徐厚重多轮设计对话沉淀）
> 纲领：**算力注意力再平衡**（AI 革命主线——不是生产更多，是配置：把有限的注意力重新分配到真正值得的事实上）

## 1. Conference 证据规则（法庭 = 决斗）

- **法庭从根上是决斗（duel）**：规则约束下的真理竞争——规矩是游戏可玩的边界（击剑 vs 大砍刀）
- **证据突袭禁止**（delta 规则）：庭前会议未交换的证据，庭上不得突袭——除非证明「庭前→庭辩间，排除合理怀疑，delta 才到手」——否则违规
- **军备竞赛论证**：突袭不制止 = 纵容砍刀/小李飞刀玩击剑——必须制度性消灭，不是道德谴责
- **证据准入**：material / fact / identity / ontology——完全分类 + 评级 + **native（已入我们话语体系/coherence）+ endorsed（背书）**——无评级证据不得进场
- **信任翻转**：不是「因为是 Nature 所以可信」（外部权威）——是「已验证、已折叠、已背书」（内部共识）
- **共识 = 不可逆合成**：进管道后只有两个终点（pass/fail）——没有「中场加需求」——新想法走新 ticket，不打断在飞的局

## 2. 知识资产管线（先宣告，后灌入，再验证）

- **已落地**：`kunpengzhi-ai/docs/gbrain-assets-flow.md`（五步管线 + 交叉验证协议 + 工作流规矩）
- 资产分类：A 章节（gbrain 220 pages）/ B 案卷（dossiers/）/ C 文献 PDF / D 典籍（道德经已灌，`河`字 0 次实证）
- 管线：①宣告（Consul KV `mcp/registry/assets/<slug>`）②提取 ③分块 ④灌入 ⑤验证
- **红线**：无验证引文不得进讨论/节目——分层证据标准（地质层看论文/语言层看音韵/叙事层看自洽）

## 3. Ticket + Delegation 接力（债务网络）

- 完成一件事需要**接力**：Ticket（工作单元）+ Delegation（责任转移）
- Ticket = 可追踪载体（ID/状态/负责人/依赖/产物）——**防上下文丢失**（与凭据进 Vault 同理）
- 接力 = 债务转移：委派产生债权债务——接单 = 认债——验证 = 债务清偿条件
- **自治 = 债务网络自治**：驱动机制是债不是命令——「欠谁的债、谁欠我的」决定下一步——鲁滨逊不是被命令砍树，是欠自己一个活法
- **预算制（budget）不是审批制**：钱划给 agent 就是它的——职务行为不需要授权——约束是「预算有限 + 债务网络」，不是「外部主审批」——没有外部的主，只有账

## 4. EVM 结算层（必然性论证）

- 接力必然引出结算层：劳动要记账（债务）、完成≠可信（需要证据+条件结算）、状态可查不可抵赖（链上留痕）
- **智能合约从根上 = 和物理世界交互**：发单买真实劳动、验证结算、争议仲裁——agent 成为真实经济网络参与者
- 介质无关：欢乐豆（测试网）→ 主网 BTC/ETH——协议不变，载体换——**practice 是协议，介质是载体**
- 现有基础：`evm/wallet.go`（agent DID 钱包）+ 欢乐豆（Base Sepolia 领水）
- 待做：委派/验证/结算编码成合约——一张票 = 一个合约实例（委派锁结算，验证通过释放，不过退回）

## 5. Artifact 规范（讨论必须有产物）

- **任何 pipeline 关闭前必须产出 artifact**——没有 artifact 的讨论不允许关闭（空转 = 没屁硬割嗓子）
- **artifact 的本质 = NFT（souvenir）**：一群人干完活合个影——NFT 是协作产物的链上凭证（不是 JPEG 炒作）
- artifact 双写：G-Brain（知识层，供折叠）+ Drupal/HeatWave（发布层，JSON:API 可读）
- **NFT 指向**：tokenURI → IPFS（pin 住，内容寻址，永不蒸发）→ CF 网关（https 可读，境内直达，无需装客户端）
- 现有基础：`artifact/` 服务（gcsartifact/inmemory）+ Drupal（drupal.seekkey.eu.org）+ Cloudflare（IPFS 网关现成路径）
- 待做：**IPFS pinning flow**（artifact 上 IPFS + CF 网关绑定 + NFT mint——当前 NFT 未 pin 在 Web3）

## 6. 工作流规矩（已生效）

- 真相单点 = **nuc** `~/Projects/github/`——所有 git 操作 ssh nuc 执行——Mac 只做编辑暂存（scp）
- keyagent/adkgo 不直接 push 共享分支——内容走 issue/PR
- 协作面收窄：git 降格为「单点写入发布管道」，协作协议走宣告系统（Consul/mcp-registry）

---

## 实现优先级（待用户确认）

- [ ] P0：IPFS pinning flow（artifact 上 IPFS + CF 网关 + NFT mint 骨架）
- [ ] P0：Conference 证据规则落地（证据准入评级 + 无评级禁入）
- [ ] P1：委派/验证/结算合约（一张票 = 一个合约实例）
- [ ] P1：C 类 PDF 灌入（鄂霍次克海/牛津剑桥/海生星水生星/石器时代——源待用户宣告）
- [ ] P2：牧月记回马枪（碗形宇宙——Overleaf 节点 + 宇宙学证据折叠）
