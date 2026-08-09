# KeyAgent 发布链路：运维清单（2026-08-09 修订）

本文件是「push → CI → Package Registry → Nomad 分发 → 19 节点」全链路的运维与故障排查指南。
2026-08-09 曾发生整条链路静默断裂（runner 不认领 / 上传 404 / label 不匹配），以下是当时修复的结论 + 防复发规则。

## 完整链路

```
代码 push main
  → CI: build-k-agent.yml（runs-on: nuc，NUC Factory）
  → nuc 本地 Go 编译 5 架构（linux-amd64/arm64/armv7、darwin-arm64、windows-amd64）
  → 上传 Gitea Package Registry: /api/packages/seekkey/generic/k-agent/{VERSION|latest}/{filename}
  → Verify step: 回读 latest 逐文件比对大小（防静默失败）
  → Nomad keyagent job（system，19 节点）start.sh 从 registry 拉对应架构 → 替换旧二进制
```

## 关键事实（踩过的坑，改前必读）

1. **Gitea 1.27 只认 `/api/packages/` 前缀**——`/api/v1/packages/` 的上传路由已被移除（实测 404）。
   build workflow 与部署脚本必须用 `/api/packages/`。**禁止**写回 `/api/v1/packages/`。
2. **runner label 不带 `:host` 后缀**——注册 `nuc:host` 后 Gitea 端只记录 `nuc`，job 要 `nuc:host` 匹配不上（srv29062 纯 label 是能接的对照组）。
   当前 nuc-runner 注册 label 为 `nuc`（config: `/home/ben/.runner-new.yaml`），workflow `runs-on: nuc`。
3. **runner 必须由 systemd 托管**——裸 `nohup`/`setsid` 进程会在 ssh 会话关闭时被杀（实测）。
   服务：`gitea-runner.service`（nuc），ExecStart 用 `/home/ben/.runner-new.yaml`。
4. **runner 注册状态会失配**——Gitea 升级/迁移后，旧注册（config 里的 registration token）可能「declare successfully 但收不到任务」。
   症状：job 永远 queued、runner_id: 0、runner 日志无 fetch task。
   修复：重新生成 registration token 并重注册 runner（见下）。

## Runner 重注册流程（故障时用）

```bash
# 1. 生成新 registration token（任意有权限的 token）
curl -X POST "https://gitea.capitaltrain.cn/api/v1/repos/seekkey/key-agent/actions/runners/registration-token" \
  -H "Authorization: token <TOKEN>"

# 2. 生成 config + 注册（labels 从 config 读，命令行 --labels 会被忽略！）
cd /home/ben
gitea-runner generate-config > /home/ben/.runner-new.yaml
# 编辑 .runner-new.yaml: labels 段改为纯 "nuc"
gitea-runner register --no-interactive \
  --instance https://gitea.capitaltrain.cn --token <REG_TOKEN> \
  --name nuc-runner -c /home/ben/.runner-new.yaml

# 3. systemd 指向新 config 并重启
sudo sed -i "s|--config .*|--config /home/ben/.runner-new.yaml|" /etc/systemd/system/gitea-runner.service
sudo systemctl daemon-reload && sudo systemctl restart gitea-runner
```

验证：`sudo journalctl -u gitea-runner -n 5` 应出现 `declare successfully`；dispatch 一个 job 后应有 `task NNN repo is ...` 日志。

## 升级 Checklist（任何 Gitea/runner 升级前必过）

1. [ ] 升级后：`curl -X POST .../actions/runners/registration-token` 能生成 token（actions 活着）
2. [ ] runner 重启后 `declare successfully`
3. [ ] dispatch 一个 build job，确认 `task NNN` 出现且 run 状态从 queued → running → success
4. [ ] 确认 registry latest 文件时间戳更新（`GET /api/v1/packages/seekkey/generic/k-agent/latest/files`）
5. [ ] 确认 nomad keyagent allocation 全部 running
6. [ ] **如 Gitea 主版本升级**：验证 generic 包上传前缀仍为 `/api/packages/`（v1 前缀曾 404）

## 节点替换机制（分发不是实时推送）

- start.sh 是 Nomad template（ChangeMode: restart），**任务启动/重启时**从 registry 拉 `latest` 并覆盖
- 新包生效 = `nomad job restart keyagent`（或节点重启自动拉新）
- 部署脚本 deploy-agents.yml 覆盖德国节点（srv29062/zhuyu），其余节点靠 restart job

## 防复发机制（2026-08-09 已落地）

1. ✅ build job 增加 Verify step：上传后回读 registry 比对大小，不一致即红
2. ✅ runner 由 systemd 托管 + 重注册（本文件第 4 条）
3. ✅ 升级 Checklist（本文件上一节）
4. 冒烟演练：发布后手动 `nomad job restart keyagent` + 抽查节点日志（尚未自动化，见 TODO）
