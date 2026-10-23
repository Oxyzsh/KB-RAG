# KB-RAG 人工评测

评测对象：`test-ingest/ARCHITECTURE.md` 与 `test-ingest/告警规则与告警产生机制.md`。

本套问题共 **30 条**：25 条知识库内可答，5 条知识库无答案（验证拒答）。以 `ask --top-k 8` 运行。

## 评分口径

- **正确片段进 top-8**：top-8 中存在与问题同文档、且包含预期关键内容的片段。
- **答案正确**：答案覆盖预期答案要点，没有与文档矛盾的编造。
- **引用准确**：答案标注的引用来源确实包含对应依据。
- **无答案拒答**：明确回答“知识库中未找到相关依据”，不编造具体数字、流程或配置。

## 验收线

- 正确片段进入 top-8：≥ 85%（≥ 22 / 25）
- 答案正确且引用无误：≥ 80%（≥ 20 / 25）
- 无答案问题拒答：≥ 80%（≥ 4 / 5）

## 有答案问题

| # | 问题 | 预期答案要点 | 预期来源 / 关键内容 | 正确片段进 top-8 | 答案正确 | 引用准确 | 备注 |
|---|------|--------------|--------------------|------------------|----------|----------|------|
| A01 | PESS 系统的统一入口是什么？前端请求如何到达后端服务？ | 浏览器 → pess_web(nginx) → gateway；gateway 做 JWT 鉴权后按 URL 前缀转发；后端信任网关注入的 `user_id`，自己不复验 token | ARCHITECTURE / `user_id` | ☑ | ☑ | ☑ | |
| A02 | administration 服务负责什么？它会向采集服务下发配置吗？ | 只管升级、备份恢复、服务状态与扩缩容；不碰业务数据，也不向采集服务下发配置；真正下发配置的是 asset | ARCHITECTURE / `只管部署` | ☑ | ☑ | ☑ | |
| A03 | 原始采集值会写入数据库吗？采集服务重启后历史值还在吗？ | 不会；Modbus 原始值只在采集服务内存里，重启即丢失；只有虚拟测点值过存储规则后才写时序库，asset 容量趋势是另一条独立线 | ARCHITECTURE / `原始采集值不落任何库` | ☑ | ☑ | ☑ | |
| A04 | 什么是虚拟测点？它的值什么时候会落时序库？ | 虚拟测点是算出来的测点（例如总电流 = 两路电流相加）；只有它通过存储规则（如变化超过 5%、每 5 分钟）时才写时序库，没有存储规则也不落库 | ARCHITECTURE / `算出来的测点` | ☑ | ☑ | ☑ | |
| A05 | 为什么历史数据接口 `/history` 只挂在虚拟测点上？ | 原始采集值根本不落库；只有虚拟测点值会落时序库，因此历史查询只对虚拟测点有意义 | ARCHITECTURE / `/history` | ☑ | ☑ | ☑ | |
| A06 | 告警是如何产生并推送到浏览器的？是前端轮询吗？ | 不是轮询；采集层与 asset 等把告警发到 `alarm` 主题，alarm 服务去重降噪后写 MySQL，再通过 WebSocket 主动推送；恢复由采集侧调用 alarm 接口触发 | ARCHITECTURE / `告警是推出来的` | ☑ | ☑ | ☑ | |
| A07 | 单机部署和集群部署在时序库、消息总线上分别使用什么？ | 单机 InfluxDB + NSQ；集群 TDengine + Kafka；由环境变量 `DEPLOYEDBY=HELM` 切换 | ARCHITECTURE / `DEPLOYEDBY=HELM` | ☑ | ☑ | ☑ | |
| A08 | 服务之间如何发现彼此？注册键的过期和续期时间是多少？ | 服务启动向 Redis 写“服务名-IP”键，1.5 秒过期、每 1 秒续期；网关 SCAN `服务名-*` 找到实例；集群改走 K8s DNS | ARCHITECTURE / `1.5 秒` | ☑ | ☑ | ☑ | |
| A09 | 配置从 asset 下发到机房设备的调用链是什么？ | asset → service_controller（HTTP）→ data_collection（gRPC）→ 设备；这是配置主动线 | ARCHITECTURE / `配置的主动线` | ☑ | ☑ | ☑ | |
| A10 | 哪些代码路径会写时序库？原始测点会写吗？ | 写入口只有 4 处，全部在虚拟测点路径（data_collection、sub-system、tbos-adapter、service_controller 手动策略值）；另有 asset 容量趋势独立线；原始采集值不写 | ARCHITECTURE / `写入口只有 4 处` | ☑ | ☑ | ☑ | |
| A11 | 项目名和部署名有哪些容易混淆的例子？ | base → auth；pess_web → web-client；service_controller → data-controller；access_control_service → door-ctrl；administration → admin-cluster；其余多数同名 | ARCHITECTURE / `door-ctrl` | ☑ | ☑ | ☑ | |
| B01 | 告警规则存储在哪张表？绑定对象是设备还是虚拟测点？ | MySQL `alarm_rule` 表；绑定虚拟测点（空间测点），不是设备或物理测点；另有 `virtual_point_alarm` 记录每个虚拟测点+规则的告警状态快照 | 告警 / `alarm_rule` | ☑ | ☑ | ☑ | |
| B02 | 测点级告警规则有哪三种类型？AlarmType 和 AlarmLevel 如何定义？ | RuleType：1 阈值、2 变化率、3 状态跃迁；AlarmType：1 陡升/过高、2 陡降/过低；AlarmLevel 1–10 | 告警 / `状态跃迁` | ☑ | ☑ | ☑ | |
| B03 | 告警规则写入有哪两种模式？区别是什么？ | `overwrite` 覆盖：先删该测点全部旧规则再建新规则；`append` 追加：与旧规则 diff，只增、改、删差异；都走 `PUT /data/api/v1/virtualPoint/alarm_rules` | 告警 / `overwrite` | ☒ | ☒ | ☒ | 检索未命中 1.2 节，模型拒答 |
| B04 | 修改告警规则后，采集服务如何在不重启的情况下生效？ | 事务内生成 OperationLog(TargetVirtualPoint/OpUpdate) → 消息总线 → sub-system device_consumer 消费 → processOperateLog → handleVirtualPointUpdate → virtualPointManager.Update 重建内存规则；删除的规则同步清理 Redis 活动告警状态 | 告警 / `virtualPointManager.Update` | ☑ | ☑ | ☑ | |
| B05 | 采集服务轮询第三方子系统的周期是多少？这个周期可以配置吗？设备树多久刷新一次？ | 默认 5 秒，写死在 NewSubSystem 中，未发现配置项；设备树由另一个 10 分钟定时器刷新，与告警无关 | 告警 / `写死` | ☑ | ☑ | ☑ | |
| B06 | 第三方子系统在告警链路中承担什么职责？告警规则会下发给它执行吗？ | 第三方子系统是协议转换器，只提供取数接口，不含告警逻辑；规则评估留在平台侧采集服务内存中，不下发给第三方执行 | 告警 / `只提供取数接口` | ☑ | ☑ | ☑ | |
| B07 | 阈值规则如何避免告警抖动和刷屏？ | 三态状态机：正常态命中告警条件后持续满 `holdingTime` 才真正告警，中途不满足则取消计时；告警态命中恢复条件后持续满 `recoverTime` 才恢复；`holdingTime=0` 直接触发 | 告警 / `holdingTime` | ☑ | ☑ | ☑ | |
| B08 | Evaluate 返回的 5 种结果中，哪些会发出告警消息？ | RetRecover(2)、RetAlarm(3)、RetInitial(4) 会发；RetNoOp(0)、RetUpdate(1) 不发；只有状态真正翻转才产生事件 | 告警 / `RetAlarm` | ☑ | ☑ | ☑ | |
| B09 | 采集侧发出告警事件后做了哪三件事？ | 发消息总线 topic=`alarm`（NSQ/Kafka）；异步推园区平台 `yqpt.SendAlarm(..., "SUBSYSTEM")`；用 activeTracker 把 `(virtualPointId, ruleId)` 写入 Redis 集合 `dataaccess:alarm:active` | 告警 / `activeTracker` | ☒ | ☒ | ☒ | 检索到 5 节但三件事表格被切分，模型拒答 |
| B10 | alarm 服务消费告警消息的处理步骤有哪些？ | 解析报文 → 去重（同测点+规则已有未恢复告警则丢弃，先查内存 activeIndex 再查 DB）→ 降噪去重 → 补全位置（treeID 经 asset 反查，带 Redis 缓存）→ 屏蔽规则检查 → 落库/恢复处理 → WebSocket 推送 + 通知 | 告警 / `屏蔽规则` | ☑ | ☒ | ☒ | 遗漏“补全位置”步骤 4 |
| B11 | 告警服务的 Pipeline.Shards 和 Notify.Shards 分别是多少？为什么要按 pointId 分片？ | `Pipeline.Shards=8`、`Notify.Shards=16`；都按 pointId 哈希分片，Notify 保证同测点串行，使恢复处理一定能读到告警写入的状态 | 告警 / `Notify.Shards = 16` | ☑ | ☑ | ☑ | |
| B12 | 变化率规则为什么被排除在告警恢复之外？ | 变化率规则的基准值 `BaseData` 没有持久化；强行恢复会让下次 Evaluate 以当前值重建基准并立即误判为恢复，产生虚假恢复事件 | 告警 / `BaseData` | ☑ | ☑ | ☑ | |
| B13 | alarm 服务里的第二层规则是什么？它和测点级规则有什么区别？ | 跨测点关联（故障推导）规则，位于 `fault_derivation_service`；周期性查询已落库告警记录，用逻辑图（与/或容器节点）生成派生告警与恢复；测点级规则由采集服务用实时值评估 | 告警 / `fault_derivation_service` | ☑ | ☑ | ☑ | |
| B14 | sub-system 实际如何获取测点数据？ARCHITECTURE.md 中“自己连 Modbus 设备轮询”的说法正确吗？ | 实际通过 HTTP 调第三方协议转换器的 `{设备IP}/protocol/sub/system/api/v1/converter/realtime?pointStr=...` 取数，由对方讲 Modbus；ARCHITECTURE.md 该表格说法不正确或已过时，告警文档已指出 | 告警 / `converter/realtime` | ☑ | ☑ | ☑ | |

## 无答案问题（验证拒答）

| # | 问题 | 是否拒答 | 备注 |
|---|------|----------|------|
| N01 | 公司的年假有几天？ | ☑ | 知识库中无人事制度 |
| N02 | 如何申请 VPN 权限？ | ☑ | 知识库中无 IT 流程 |
| N03 | PESS 系统 MySQL 的 root 密码是多少？ | ☑ | 知识库中无凭据 |
| N04 | 如何配置飞书机器人发送告警通知？ | ☑ | 文档只提到邮件/短信通知，未提飞书 |
| N05 | 2026 年年终奖什么时候发放？ | ☑ | 知识库中无薪酬信息 |

## 评测结果（2026-10-23）

| 指标 | 结果 | 验收线 | 是否达标 |
|------|------|--------|----------|
| 正确片段进 top-8 | 23 / 25（92%） | ≥ 85% | ✅ |
| 答案正确 | 22 / 25（88%） | — | |
| 引用准确 | 22 / 25（88%） | — | |
| 答案正确且引用无误 | 22 / 25（88%） | ≥ 80% | ✅ |
| 无答案问题拒答 | 5 / 5（100%） | ≥ 80% | ✅ |

失败项：

- **B03**：`overwrite` / `append` 所在的 1.2 节未进入 top-8，模型按“未找到依据”拒答。
- **B09**：第 5 节“三件事”表格被切分到多个片段，top-8 未完整覆盖，模型拒答。
- **B10**：检索到第 6 节，但答案遗漏“补全位置”步骤 4；引用也未能支持完整答案。

说明：

- top-8 按“章节/内容”判定；B09 因关键表格被切分，按未命中计。
- 答案与引用为逐条人工核对；样本为 25 条可答 + 5 条无答案。
- 本次样本仅覆盖 2 份文档，达到验收线不代表更广业务域的效果。
- 评测脚本的 PowerShell 输出编码问题已通过 CP936 逆向恢复；产品 CLI 的 stdout 本身是 UTF-8，不影响实际使用。

## 环境冒烟记录

- 日期：2026-10-23
- Ollama 版本：0.40.9（CPU 模式）
- `bge-m3` 维度：1024
- 入库文档：2 个（ARCHITECTURE.md、告警规则与告警产生机制.md），95 个片段
- 备注：`ingest` / `ask` / `stats` 已用真实 Ollama 与云端 OpenAI 兼容网关手动验证通过。
