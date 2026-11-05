# Server HTTP Protobuf 接口

[server.proto](server.proto) 定义 `/v1/server/online`、`/v1/server/push`、
`/v1/server/status`、`/v1/server/config`、`/v1/server/user` 和
`GET /v2/server/{server_id}` 的可选 protobuf 传输。请求 body 使用
`Content-Type: application/protobuf`，响应使用 `Accept: application/protobuf` 协商；未指定
时为 JSON。身份及 secret 仍在既有 query 参数中，成功 protobuf 信封 `code=200`。
`/v1/server/config` 已移除：它不能承载 v1 routing，固定返回 HTTP 426；节点改用 `/v2/server/{server_id}`。

## Ordered routing v1/v2

- `POST /v1/server/online`
- `POST /v1/server/push`
- `POST /v1/server/status`
- `GET /v1/server/config`
- `GET /v1/server/user`
- `GET /v1/server/alivelist`
- `POST /v1/server/device`
- `GET /v2/server/{server_id}`

`GET /v2/server/{server_id}` 必须携带 `X-PPanel-Routing-Version: 1`；缺失或不支持时在
ETag/304 前返回 HTTP 426、业务码 `140001`。响应 `Vary: Accept, X-PPanel-Routing-Version`。
JSON 和 protobuf 均返回完整 routing v1，不再下发顶层 block/outbound。旧 protobuf 字段编号
保留但不填充，旧节点必须同批升级。零协议时 JSON 的 `protocols` 为 `[]`，`total` 等于数组长度；周期 0 使用默认，其他值为 1–86400 秒。

- `QueryServerProtocolConfigData.routing=12`，`Outbound.id=44`；其余既有编号不变。
- `RoutingPolicy`：version=1、outbounds=2、rules=3、default_outbound_id=4、mode=5、source=6、
  source_server_id=7、source_revision=8、local_transition_revision=9。
- `RouteRule`：id=1、**optional** enabled=2、remark=3、kind=4、values=5、outbound_id=6。
  RouteKind：unspecified=0（非法）、domain=1、ip=2、protocol=3。每条一个 kind、values 非空；
  即使 disabled 也检查格式和引用。自定义出站的旧匹配数组必须为空。
- 节点身份 mode 为 `ordered-v1-inherited` 或 `ordered-v1-local`。没有 policy、未知版本/类型、
  坏引用或混合旧表示均不得执行。默认出口不可省略。
- `Outbound` 保留协议、地址、凭据、TLS、传输、插件、ALPN、XHTTP、encryption、multiplex、
  raw settings 等既有字段；插件参数不能无损转换为 protobuf `Value` 时拒绝，而非改变数值。

`GET /v1/server/alivelist` 返回当前 server/protocol 可服务订阅的全局在线来源 IP 数，
JSON 为 `{"alive":{"订阅ID":数量}}`，无在线设备时为 `{"alive":{}}`。不使用 ETag，
避免在线数据过期后仍沿用旧计数。用户 ID 与 `GET /v1/server/user` 的订阅 ID 一致。

`POST /v1/server/device` 接收 JSON `{"uid":订阅ID,"ip":"来源IP"}` 或
`ClaimServerDeviceRequest`，返回 `{"allowed":true/false,"ttl_seconds":300}` 或
`ClaimServerDeviceResponse`。身份由 query 参数确定，body 不能覆盖 server/protocol。
Redis 原子检查同订阅的独立 IP 数并申请/续期五分钟租约；同 IP 跨节点或协议去重，
IPv4-mapped IPv6 归一化为 IPv4。设备限制为 0 时不限名额，其他情况按当前用户列表
中的设备限制准入；已获名额的 IP 可续期。在线上报也续期租约，失联条目自动过期。
单次在线上报最多 10000 项，去重后全部续期；分批上报按 IP 合并，不覆盖前一批。

所有成功的 Protobuf 信封均使用 `code = 200` 和 `message = "success"`。失败响应
使用 `Result`；带业务数据的成功响应使用强类型 `data` 字段。
`GET /v2/server/{server_id}` 定义了与既有 PPanel 节点对接一致的 DNS、出站和
协议消息。只有 `plugin_options` 使用 `google.protobuf.Value`，以保留对象和
SIP003 字符串两种插件参数形式。

`PushServerStatusRequest` 中 routing_version=5、routing_applied=6、routing_pending=7、
routing_error=8、instance_id=9、routing_sequence=10（uint64，0 表示缺失）；JSON 名称相同。`RoutingIdentity` 为 mode=1、source=2、
source_server_id=3、source_revision=4、local_transition_revision=5。instance_id 是节点
进程 UUIDv4，sequence 随每次进程上报单调递增，server 级状态不由单一协议覆盖。按 sequence 而非客户端时间排序：较旧或重复序号只能刷新实例 `liveness_at`，不能刷新可信内容 `received_at`；内容超过 TTL 即 unknown，多个活跃实例也始终 unknown。Redis 会有限期保留序列水位抵御跨 TTL 的延迟包，但 Redis 丢失或水位过期后不能证明过去的序号。管理端保存 desired 不依赖状态上报，不会伪造 applied；只有节点成功切换核心和监听并上报与当前 desired 完整身份匹配的 applied 才代表应用。

## REALITY target PROXY protocol

`ServerProtocol.reality_xver=66` 是独立于 `accept_proxy_protocol` 的 target-side PROXY
版本：0 禁用、1 为 PROXY v1、2 为 PROXY v2。旧字段编号不变；缺失值为 0。

## 切换与回滚

使用 `02180_ordered_node_routing` 之前，停止全部旧服务和写入者，备份数据库与旧三端版本。
启动前对 02179 全局及每个服务器有效旧配置执行只读等价转换预检；任何阻断在 DDL 前失败。
迁移与确定性配置写入完成后才启动服务，运行时要求全局 v1 存在。旧路由列只作回滚材料，
不参与运行时下发或管理写入；旧管理字段返回 422。回滚应恢复升级前 DB 备份及原三端版本，
不是在 02180 数据库启动旧二进制。详细约束见
[ordered-node-routing.md](../../../docs/design/ordered-node-routing.md)。

修改 schema 后，在仓库根目录运行 `make proto` 重新生成 Go 绑定。它要求
protoc 版本与 CI 一致（`make -s print-PROTOC_VERSION`，即 libprotoc 3.21.12），
并按 go.mod 中锁定的版本构建 protoc-gen-go，保证生成文件头与提交内容一致。
CI 的 `make proto-check` 会重新生成并在与提交的 `server.pb.go` 不一致时失败。
