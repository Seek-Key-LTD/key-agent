# ADR 0001 — A2A 身份校验：用 userinfo 结果缓存消除逐请求 IdP 依赖

- **状态**：Accepted
- **日期**：2026-09-29
- **相关**：`auth/middleware.go`、`auth/authentik.go`、`docs/01_NOMAD_AUTHENTIK_VAULT_IAC_TEMPLATE.md`

## 背景

A2A 服务端用 `AuthentikUserInfoMiddleware` 校验 incoming Bearer token：每个请求都同步
调 Authentik 的 userinfo 端点做 introspection。这让 **Authentik 成为整张 A2A 网状拓扑的
单点**——IdP 一挂（升级、抖动、网络分区），所有节点的入站认证在同一时刻一起瞎，
是典型的相关性故障（correlated failure），不是普通的可用性抖动。

## 决策

保留 userinfo introspection 作为**验真原语**，但对它的**正向结果**加一层进程内短 TTL
缓存（默认 60s），把逐请求出网压成"每个 token 每 TTL 出网一次"。

规则：

- 缓存键 = bearer token 的 **SHA-256**，绝不落原文 token、绝不写日志。
- 缓存上限额外用 token 自带 `exp`（best-effort 解 payload，**不验签、不作放行依据**）收紧，
  保证缓存放行窗口 ≤ token 自身寿命（M2M token 寿命 600s）。
- 命中缓存即放行，**即使此刻 IdP 不可达**——这正是 SPOF 消除的证明。
- 冷路径（无缓存且 IdP 不可达）→ **503**；token 无效/过期 → **401**。二者语义分离，
  避免把"基础设施在抖"误报成"对端越权"。
- 负向结果（401）**不缓存**，每次重新 introspection。
- 新增 `AuthentikUserInfoMiddlewareWithCache(url, ttl, next)`；`ttl<=0` 关闭缓存退化为旧行为；
  原 `AuthentikUserInfoMiddleware` 签名与 `CallerIdentity(ctx)` 契约保持不变。

## 为什么不是"本地 JWKS 验签"（被否方案）

直觉方案是：改成用 Authentik 的 JWKS 在本地验 JWT 签名，彻底不出网。**这条路在
Authentik M2M 场景下不成立**：

实测 Authentik 对 `client_credentials` 签发的 token 是 **HS256（对称 HMAC）**
（`{"alg":"HS256"}`），不是 RS256/ES256。HS256 的验签密钥 = 签发密钥 = 该 provider 的
`client_secret`。**收方持有的是别人的 secret，拿不到发送方的**，因此只有 Authentik
自己（掌握全部 provider secret）能验。除非把所有 seat 的 secret 下发到每个节点
（直接违反"异挂/不共享"红线），本地验签无从谈起。

所以 introspection 是正确原语，问题只在"逐请求同步出网"，用缓存治理即可。

### 备选：让 Authentik 改发 RS256 token？

把 M2M 切到非对称签名能让本地 JWKS 验签成为可能，但需要改 provider 级配置、
每 seat 的 key 管理、以及验签侧的 kid/轮换处理，改动面远大于收益，且缓存方案已能把
逐请求依赖降到 TTL 级、把相关性故障窗口压到很小。**暂不做**，留作后续若真要彻底
去 IdP 热路径时的方向。

## 后果

- **利**：Authentik 抖动时已验真的 seat 间调用不再成片掉线；出网调用量按 TTL 摊薄。
- **代价**：token 被主动撤销后，最长有 ~60s 的缓存放行窗口（M2M 撤销本就低频，
  窗口远小于 token 600s 自然寿命）。需要更小窗口可经 `WithCache` 调低 TTL。
- 缓存为**进程内**、每节点各一份，不跨节点共享，不引入共享状态，符合"同核异挂"。
- 补齐了 `middleware.go` 缺失的 Apache license header（过 `goheader`）。

## 验证

- `go test -race ./auth/` 9 个用例全过，含 `IdPDownButCached`（暖缓存 + IdP 下线仍放行）
  与 `IdPDownColdFail503`（冷路径 503）。
- `golangci-lint run ./auth/...` 0 issues；`gofmt`/`go vet` 干净；`go mod tidy -diff` 无 drift（仅标准库）。
