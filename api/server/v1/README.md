# Server HTTP Protobuf 接口

[server.proto](server.proto) 定义 `/v1/server/online`、`/v1/server/push`、
`/v1/server/status`、`/v1/server/config`、`/v1/server/user` 和
`GET /v2/server/{server_id}` 的可选 protobuf 传输。请求 body 使用
`Content-Type: application/protobuf`，响应使用 `Accept: application/protobuf` 协商；未指定
时为 JSON。身份及 secret 仍在既有 query 参数中，成功 protobuf 信封 `code=200`。
`/v1/server/config` 已移除：它不能承载 v1 routing，固定返回 HTTP 426；节点改用 `/v2/server/{server_id}`。

## Ordered routing v1（唯一节点路由）

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

修改 schema 后，使用以下命令重新生成 Go 绑定：

```sh
PATH="$(go env GOPATH)/bin:$PATH" \
  protoc --go_out=paths=source_relative:. api/server/v1/server.proto
```
