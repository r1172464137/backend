# 有序节点路由：停机切换到 routing v1

状态：cutover-only 设计。升级后节点运行时只有一套有序路由，不再运行旧版“屏蔽优先”路径。
本文件描述源码契约，不构成上线授权；生产变更、发布与推送需要单独授权。

## 目标与范围

出站连接独立定义，域名、IP、协议的路由规则共用一个有序数组。首条匹配生效，屏蔽也是
`block` 出站；管理员可将 `geosite:google-cn → direct` 放在 `geosite:cn → block` 前。
第一版包括域名/GeoSite/RE2、IP/GeoIP/CIDR、BitTorrent、启停、顺序、备注、明确默认出口、
全局配置和服务器整包覆盖。DNS、IPStrategy、拉取/上报周期不属于路由包。

这是节点端路由，不修改 Clash 订阅模板。客户端若先将请求本地直连，请求不会进入节点，节点
路由不能改变客户端结果；必须另行修改并验证客户端规则集和只包含节点的策略组。`direct`
是节点服务器直出，不是用户设备直连。

## 一、唯一运行时模型

```json
{
  "version": 1,
  "outbounds": [],
  "rules": [
    {"id":"rr_11111111-1111-4111-8111-111111111111", "enabled":true,
     "remark":"Google CN", "kind":"domain", "values":["geosite:google-cn"], "outbound_id":"direct"},
    {"id":"rr_22222222-2222-4222-8222-222222222222", "enabled":true,
     "remark":"CN block", "kind":"domain", "values":["geosite:cn"], "outbound_id":"block"}
  ],
  "default_outbound_id": "direct"
}
```

- 内置 `direct`、`block`；自定义出站 `ob_<UUIDv4>`，规则 `rr_<UUIDv4>`。连接名称可编辑，
  引用使用 ID；ID 不随排序改变。规则和默认出口不能引用不存在的连接，禁用规则也校验引用。
- 一条规则只有 `domain`、`ip`、`protocol` 中一种 `kind`；同条多值为 OR，规则数组按顺序首条
  命中。`enabled` 必须明确出现，false 不等于缺失；`values` 非空。空规则数组合法，默认出口仍必填。
- 裸域名规范为精确 `full:`，`suffix/domain` 为后缀；GeoSite、GeoIP、CIDR 和 RE2 复用既有
  校验。protocol 当前只支持 `bittorrent`。只在同一规则内按顺序去重，不跨规则重排或去重。
- 自定义出站保留既有协议、地址、认证、TLS、传输、插件参数、原始 settings 等字段；新版
  出站的 `rules`、`ip_rules`、`protocol_rules` 必须为空。未知版本、坏引用或无法编码均 fail closed。
- 固定上限：128 个自定义出站、512 条规则、每条至多 1024 个输入值、单个规范值至多 4096
  UTF-8 字节、全包至多 8192 个值和 256 KiB 规范匹配文本，最终 JSON 不超过 1 MiB（含边界）。
  备注最多 200 Unicode 字符。输入数组和字节也在规范化之前受限。HTTP 包含 envelope、DNS、
  protocols；部署时还须检查代理限额和 MySQL `max_allowed_packet`。

### 继承与身份

数据库全局 `Routing` 必须存在，所有服务器始终执行 routing v1。服务器 `routing IS NULL` 表示
整包继承当前全局策略（`global-v1`）；非 NULL 表示完整本地包（`local-v1`）。不存在运行时
legacy、局部继承两套路由或 hybrid 状态。本地空 `rules: []` 是有效覆盖，不等于继承。

节点 wire 为兼容既有节点 v1 实现，分别使用 mode 字符串 `ordered-v1-inherited` 和
`ordered-v1-local`，它们只是上述两种状态的 wire 名称。候选完整身份为
`(mode, source, source_server_id, source_revision, local_transition_revision)`。全局 source 为
`global`，server ID 为 0；本地 source 为 `server`，server ID 为本服务器 ID，source revision
等于 local transition revision。不要只比较裸 revision。

## 二、停机升级与一次性转换

升级必须先停止所有旧 backend、所有写入者及相关节点，并备份数据库及旧三端版本；再同批切换。
应用自身在 HTTP、队列和 scheduler 启动之前完成 preflight、迁移和初始化。它无法证明外部旧
进程已停止，这一前提由运维独立保证；禁止滚动混用新旧 backend 或新旧节点。

对于已有数据库，当前 schema 必须是干净的 `02179`。新进程在调用 `schema.Up` 之前，只读
`system` 和每个服务器覆盖，计算全局和每个服务器的**实际有效旧配置**并转换。只覆盖 DNS 或
IPStrategy 的服务器不创建本地路由；有任一旧 block/outbound 局部覆盖的服务器转为完整 local
v1，之后不再传播全局路由。四种旧继承组合均先按旧有效配置计算。

预检转换严格保留旧 builder 的业务规则顺序与 OR：

```text
block_protocols → protocol/block
block_ips       → ip/block
block           → domain/block
每个 outbound 按原顺序：rules → domain，ip_rules → ip，protocol_rules → protocol
默认 → direct
```

空类别不产生规则；相邻三类分别产生规则，不能合成跨类型 AND。转换不自动插入 Google 例外，
也不调整、跳过或改名既有规则。ID 根据作用域与原对象/类别位置稳定生成 UUIDv4，重试结果相同。
重复名称、`Default`/`block`/`dns_out` 标签冲突、非法规则、未知或无法构建的协议和无法确认
等价的 raw settings/stream settings 都是对象级阻断，预检失败时**不得执行 02180 DDL**。
旧 builder 忽略但新版会消费或拒绝的插件、ALPN、XHTTP、multiplex 等配置也不能静默转换。enabled 监听器的 XHTTP mode、extra 字段类型、范围及嵌套传输在预检和在线写入前校验；无法静态证明的复杂 download settings 保守阻断，不能当作节点不支持该配置的结论。
REALITY `spider_x` 的预检和管理写入先于节点执行与同批节点一致的 URI/数值边界：URI 最多 4096 字节、最多 32 个查询键，`p≤4096`、`c≤8`、`t≤16`、`c×t≤64`、`i≤30000ms`、`r≤5000ms`，拒绝重复参数和倒序范围；嵌套 raw stream/XHTTP 的 REALITY 设置同样校验。节点仍须独立防护。可可靠验证的 custom freedom/blackhole raw settings 保留为独立自定义出站，不折叠为内置出口。

预检通过后执行 `02180_ordered_node_routing`：MySQL `system.value` 加宽为 MEDIUMTEXT；
server override 增加 nullable `routing`（MySQL MEDIUMTEXT，PostgreSQL TEXT）和非空
`routing_revision`。全局新增 `Routing`、`RoutingRevision`、`RoutingCutover`。
迁移初始 marker 是 `pending`；组装层分别以 platform 和 network 各自事务写入确定性包，核对后
才将 marker 改成 `complete`。不新建跨模块事务。初始 global revision 为 1，有本地包的服务器
初始 local revision 为 1；无本地包保持 NULL/0。旧字段仅保留在数据库作升级前备份材料，运行时
仓储不读取，也没有管理写路径。

崩溃后若数据库是 02180 且 marker 仍为 `pending`，启动前重新只读预检并用同一确定性计划
核对/补齐，最后标记 complete；遇到不一致或 dirty migration 必须停止并恢复备份，不能猜测或
强制改 schema 版本。若 marker 完成但全局包缺失/损坏/版本未知，启动和下发均 fail closed。
全新空库没有旧配置，可迁移后创建空规则、默认 direct 的全局 v1。初始化提交也独立验证空库，不能仅依赖测试连接接口；空迁移表或首迁移前中断只有在确认没有业务对象后才恢复。setup 提交全程持有配置文件锁和数据库安装锁；安装先以 0600 原子保存配置及一次性管理员密码哈希，seed 成功或崩溃恢复后原子移除；不保存明文密码。

### 回滚

回滚是停止所有新进程、恢复**升级前数据库备份**、恢复升级前的 backend/frontend/node 版本，
并验证一致性；不可在已切换数据库上启动旧业务二进制，不提供混合版本或兼容旧二进制运行路径。
正常回滚不执行 destructive migration down、不修改 `schema_migrations`，也不丢弃旧备份。
MySQL DDL 非事务性；中断时应先检查 dirty/version 与实际列，再恢复备份，不盲目 force/retry。

### DNS 校验

全局和服务器本地 DNS 写入、启动和 cutover 前均验证：`proto` 仅支持空值或 `udp`，`server_name` 必须为空；`address` 是合法主机名/IP，可带 1–65535 端口（省略为 53），不能含 scheme、路径、userinfo 或无效 IPv6。`domains` 使用节点路由相同规则规范化并拒绝无效 RE2；未验证的 DNS 不会静默丢弃。节点侧仍独立验证。

## 三、权威存储和管理 API

platform 拥有全局 `system`，network 拥有 `server_config_overrides`；跨模块读取通过窄只读
端口组装，不跨属主 JOIN 或事务。节点配置读取先从 DB 无缓存读取全局 revision 和包，再读取
服务器自身覆盖，再读一次全局 revision；版本变化时有界重试，失败拒绝下发，不回退进程内
`Config().Node`。普通 global/local 包写各自在属主事务内锁定并比较自身 revision，然后递增；
不再使用跨模式 advisory gate。DNS/IPStrategy 等非路由更新不递增。即使恢复整包继承，非零
local revision 元数据也保留，防止 token 被重用。

- `GET/PUT /v1/admin/system/node_config`：`routing`、`routing_revision`、`routing_mode` 和
  只读 `routing_capabilities`。routing 包本身不持久化 revision。提交新包必须带预期 global revision。
- `GET /v1/admin/server/node_config`：`global`、`override`、`effective` 和 `routing_status`，
  返回对应模式/来源/revision。`POST /v1/admin/server/node_config/update` 使用整包 `routing`、
  `routing_revision`；显式 `inherit_routing: true` 清除本地包并递增本地 revision。保存本地包或返回继承不依赖节点状态，新建、离线、未上报的服务器也可预配置；仍须通过 revision CAS 和完整引用校验。成功只表示 desired 已存，不表示节点已应用。`routing_status` 独立展示真实节点上报，未上报为 unknown、上报 pending 时仍非 applied；只有节点上报的 applied 完整身份与当前 desired 匹配才证明应用。复制的包是独立草稿；全局与本地各自 CAS，不承诺跨属主的原子自动合并。旧节点拉取仍以 426 fail closed。
- `POST /v1/admin/server/create` 成功的 `data.id` 是已提交的新服务器 ID（不返回密钥），供复制 DNS/IP/路由后续操作。创建请求不具备幂等键；发生网络中断等结果未知时应先读回确认，不能盲目重试。服务器配置 GET 和节点配置下发直接读取权威数据库；服务器配置及覆盖写入只修改服务器字段或 DNS/IP/routing，不改变用户、订阅或节点授权，因此不再失效无关的 `server:user:*` 授权列表缓存，也不产生提交后的失效失败。该用户列表缓存自身 TTL 为 5 分钟；节点/订阅/用户授权相关写路径的失败失效仍先由进程内重试，若进程崩溃，新进程在启动 HTTP/worker 前扫描 Redis 中的 indexed 和 legacy server 缓存（包括已删除服务器）、逐 ID 失效并递增 generation fence；Redis 不可用则启动失败。未重启期间正常索引缓存最多滞留 5 分钟。旧 `server:config:*` 不被 v2 或已移除的 v1 配置路径读取。Redis 故障时 routing_status 为 unknown。
- 两入口省略 `routing` 为保留，`routing: null` 不作为隐式继承。需要空业务表时提交完整 v1 包。
  stale revision 返回真实 HTTP 409：platform 业务码 `160001`、network `140002`。
- 旧路由字段（`block`、`block_ips`、`block_protocols`、顶层 `outbound` 及服务器旧继承字段）
  的任何写请求返回 HTTP 422：platform `160003`、network `140003`。这些字段不出现在新的
  管理或节点投影。旧客户端不能继续写路由；前端必须同批升级。

## 四、节点协商、wire 与状态

`GET /v2/server/{server_id}` 只下发完整 routing v1，不含顶层旧 block/outbound 字段。请求必须
携带 `X-PPanel-Routing-Version: 1`；缺失或其他版本返回 HTTP 426、network 码 `140001`，检查
发生在配置读取和 ETag/304 之前。`Vary` 包含 `Accept` 和能力头；protobuf ETag 用 deterministic
marshal。无法承载完整 v1 路由的旧 `/v1/server/config` 恒返回 426，不再执行旧配置下发逻辑。
零监听时 JSON 必须输出 `protocols: []`，不能为 null；`total` 必须严格等于 protocols 数量。拉取/上报周期 0 表示节点默认，非 0 限制为 1–86400 秒。

protobuf 保留所有旧 tag 并追加：`QueryServerProtocolConfigData.routing=12`、`Outbound.id=44`。
`RoutingPolicy` 的内容和身份为 tag 1–9；`RouteRule.enabled` 是 optional bool（tag 2）。JSON 与
protobuf 都投影所有已有出站字段和 raw settings；不能无损写入 protobuf `Value` 的插件数值被拒绝。

| 连接字段矩阵（backend DTO → JSON → protobuf → 节点 DTO） | 节点 builder 实际消费或限制 |
|---|---|
| `id`、`name`、`protocol`、`address`、`port`、`user`、`password`、`uuid`、`cipher` | ID 生成独立 tag；协议和凭据由对应连接构建器消费，未知/无法构建拒绝 |
| `security`、`sni`、`allow_insecure`、`fingerprint`、`reality_public_key`、`reality_short_id`、`spider_x` | TLS/REALITY 构建时消费并校验 |
| `transport`、`host`、`path`、`service_name`、`xhttp_mode`、`xhttp_extra` | 由所选传输消费；XHTTP 扩展仅在可用的 XHTTP 传输上生效 |
| `alpn` | 无 raw stream 时要求 TLS；legacy 等价转换会阻止原先未消费而现在会生效的值 |
| `plugin`、`plugin_opts` | 均无损往返；当前 ordered builder 拒绝非空 plugin，不承诺新增插件能力 |
| `flow`、`encryption`、`encryption_mode`、`encryption_rtt`、`encryption_ticket`、`encryption_client_padding`、`encryption_password` | flow 按协议消费；无 raw settings 时不支持的 encryption 组合拒绝，不宣称已支持新加密 |
| `multiplex`、`uot`、`uot_version` | 字段往返；不支持的 multiplex 和所有未消费的非默认 UoT 配置拒绝，包括嵌套 raw 语义 |
| `congestion_controller`、`udp_stream`、`reduce_rtt`、`heartbeat` | 保留兼容字段；TUIC 出站及其传输 dialer 均未实现并拒绝 |
| `settings`、`stream_settings` | 原始 JSON 字符串无损往返，优先用于节点连接构建；切换预检对不能证明等价的 raw 配置阻断 |
| `rules`、`ip_rules`、`protocol_rules` | 仅保留 wire 字段编号，新包连接中必须为空；实际匹配只来自有序规则表 |

`PushServerStatusRequest` 以 `routing_version=5`、`routing_applied=6`、`routing_pending=7`、
`routing_error=8`、`instance_id=9`、`routing_sequence=10` 上报；JSON 名称一致。`RoutingIdentity` 的 `mode/source/
source_server_id/source_revision/local_transition_revision` 为 tag 1–5。`instance_id` 是进程级
UUIDv4，重启后变化，不是协议 controller ID。即使没有可运行协议，也由 server 级任务上报。

Redis 以 server ID 和进程 ID 分别记录，以 Redis 接收时间判定新鲜度；超过 TTL、Redis 丢失、
旧节点缺 ID/sequence、坏报文或多个新鲜进程（包括同 NAT）均为 unknown。内容按同一 instance 的单调 sequence 排序；旧包及完全相同的重复 sequence 只刷新 `liveness_at`，不刷新可信内容 `received_at`，内容到期后即使实例仍活跃也为 unknown。同 sequence 内容冲突为 unknown，只有更高 sequence 能恢复；同毫秒或客户端时钟回拨本身不构成冲突。为抵抗跨活跃 TTL 的延迟包，Redis 在最后接收后额外保留 sequence 水位最多 24 小时，每个服务器最多保留 4096 个实例水位，超过上限 fail closed；Redis 丢失或水位到期后无法判定更早的重放。序列和 revision 不经过 Lua 浮点转换。报错有界、脱敏。只有节点在新 core 与
必要监听均成功切换后上报的 applied 身份才表示实际应用；保存、下载或 pending 均不表示成功。

## 五、节点编译与验收

节点保留 DNS 构建与 UDP/53 系统路由，按数组顺序编译启用规则，每规则只生成一个 Xray field
rule；默认出口放在 Xray outbound 第一位。自定义 ID 映射到独立 tag 命名空间，不与
`Default`、`block`、`dns_out` 冲突。HTTP CONNECT 出站仅支持 TCP，不能承诺 UDP 转发；SOCKS/SS 的 UDP 能力仍取决于上游。TUIC/WireGuard、自定义 DNS/loopback 业务出站均拒绝。backend 与同批节点 builder 的共同 fixture 位于 `internal/app/bootstrap/testdata/node-capabilities.json`，可用 `PPANEL_NODE_ROOT` 指向节点 checkout 验证，转换器允许的配置必须能通过真实节点 validator 和 builder。保持 `domainStrategy=AsIs` 和 Xray Freedom 既有私网安全
边界。坏候选不得降级旧字段或替换已应用 core；应用、pending、失败身份分离并有界重试。

验收必须覆盖：两种 DB 的 02179 → 预检失败无 DDL → 02180 转换、1 MiB、重启恢复和备份
恢复旧版本；旧 OR 顺序、四种旧继承组合、保留标签及 raw/protocol 阻断；global/local CAS 与
并发双读；422/409、空规则/false、完整 JSON/protobuf、能力先于 304、1 监听到 0；零协议、
多协议乱序、同 NAT 双实例、过期/Redis 丢失、pending/applied/error。还需固定 GeoSite fixture
证明 `www.gstatic.com` 同时属于 google-cn 与 cn，正反顺序产生不同出口，真实 HTTPS direct/
SOCKS/block 有数据面收据；如修改 Clash，另验客户端 MRS、DNS 和仅节点策略组。单元测试与
数据库演练不能代替三端实际应用回执、浏览器回读和数据面证据。
