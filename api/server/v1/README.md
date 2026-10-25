# Server HTTP Protobuf 接口

此目录的 [server.proto](server.proto) 为节点 Server 接口提供可选的
Protobuf 二进制传输，覆盖以下端点：

- `POST /v1/server/online`
- `POST /v1/server/push`
- `POST /v1/server/status`
- `GET /v1/server/config`
- `GET /v1/server/user`
- `GET /v2/server/{server_id}`

上报类请求使用 `Content-Type: application/protobuf` 发送 Protobuf body；
增加 `Accept: application/protobuf` 可请求 Protobuf 响应。未指定时仍默认使用
JSON。GET 接口只需使用 `Accept` 协商响应格式。既有的 `server_id`、`protocol`
与 `secret_key` 查询参数保持不变。

所有成功的 Protobuf 信封均使用 `code = 200` 和 `message = "success"`。失败响应
使用 `Result`；带业务数据的成功响应使用强类型 `data` 字段。
`GET /v2/server/{server_id}` 定义了与既有 PPanel 节点对接一致的 DNS、出站和
协议消息。只有 `plugin_options` 使用 `google.protobuf.Value`，以保留对象和
SIP003 字符串两种插件参数形式。

Nowhere 节点使用 `GET /v2/server/{server_id}` 获取扁平协议配置，并复用现有的
用户、状态和流量接口。服务端配置固定为 TLS，版本为 1；`network` 为 `mix`、
`tcp` 或 `udp`，ALPN 默认且只能包含一个 `now/1`。用户列表中的 UUID 直接作为
Nowhere shared key。旧版 `GET /v1/server/config` 不定义 Nowhere 配置结构。

## REALITY target PROXY protocol

Each server protocol can set numeric `reality_xver` to `0` (default, no target-side
PROXY header), `1` (PROXY v1), or `2` (PROXY v2). It is independent of
`accept_proxy_protocol`, which controls reception at the inbound listener.
For example, keep reception enabled on port 20100 behind nginx and explicitly set
`reality_xver: 2` for a REALITY target at `127.0.0.1:8443` that expects PROXY v2.
Ordinary remote targets should keep `reality_xver: 0`.

The value lives in the existing `protocols` JSON; no database migration is needed.
Omitted values on new protocols default to zero. The existing field-preserving
admin update contract preserves an omitted value on existing protocols; send
explicit `0` to reset it. Zero may be omitted in JSON responses/storage and on the
protobuf wire. Consumers must default an absent value to zero. Old consumers can
ignore this additive field. Protobuf `ServerProtocol` uses `uint32 reality_xver = 66`.
The legacy config endpoint exposes nonzero values in `security_config` for REALITY.
Non-REALITY cleanup clears this field like the other REALITY settings (Nowhere
retains its strict unsupported-option rejection). Invalid REALITY values are
rejected, including on disabled configurations; JSON values must be integers.

修改 schema 后，使用以下命令重新生成 Go 绑定：

```sh
PATH="$(go env GOPATH)/bin:$PATH" \
  protoc --go_out=paths=source_relative:. api/server/v1/server.proto
```
