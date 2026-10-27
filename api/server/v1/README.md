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

Per-protocol numeric `reality_xver` selects 0 (default, disabled), 1 (PROXY v1),
or 2 (PROXY v2) sent to the REALITY handshake target. It is independent of inbound
`accept_proxy_protocol`. Existing remote targets remain at zero. Protobuf uses
`uint32 reality_xver = 66`; JSON uses `protocols[].reality_xver` and may omit zero.
The existing protocols JSON stores the value without a migration. New omissions
default to zero; existing admin update omissions preserve the stored value,
while explicit zero resets it. Non-REALITY cleanup clears it (Nowhere retains
strict unsupported-option rejection). Invalid REALITY values are rejected even
when disabled. JSON values must be integers. Legacy config exposes nonzero values
in `config.security_config.reality_xver`.

## Structured route rules

`GET /v2/server/{server_id}` keeps the existing domain arrays `block` and
`outbound[].rules`, and adds independent IP and protocol arrays. Top-level
`block_ips` and `block_protocols` use protobuf tags 10 and 11;
`Outbound.ip_rules` and `Outbound.protocol_rules` use tags 42 and 43. Domain,
IP and protocol categories remain separate so nodes can emit OR routing rules.
Old JSON and protobuf consumers ignore these additive optional fields.

Backend and node use the same canonical grammar:

- bare and `full:` hostnames become `full:`; one terminal DNS root dot is removed, while internal or repeated trailing dots, whitespace and malformed labels are rejected;
- `keyword:` remains `keyword:` for node-side conversion, and its payload must be nonempty with no whitespace or control characters;
- `suffix:` and canonical `domain:` hostnames become `domain:` with the same hostname validation;
- `regex:` and `regexp:` become `regexp:` and must compile with Go `regexp.Compile`;
- GeoSite labels match `^[A-Za-z0-9][A-Za-z0-9_.!-]*(?:@[A-Za-z0-9_!-]+)*$`;
- GeoIP labels match `^[A-Za-z0-9][A-Za-z0-9_-]*$`;
- IP addresses and prefixes use strict `net/netip` parsing;
- the protocol allowlist is exactly lowercase `bittorrent`.

Limits apply to canonical unique values: 1,024 rules per category and 4,096
bytes per canonical rule. Structured outbounds are capped at 128, 4,096
aggregate canonical route rules and 48 KiB of aggregate canonical route text;
DNS resolver rules do not count toward that aggregate. Every final JSON value
stored in a MySQL/PostgreSQL TEXT field must remain below 60 KiB after escaping.

Migration `02179` is intentionally retained below the master `03000` band so
a database upgraded from stable does not apply the same schema change twice.
Its down migration is destructive and operator-only; production startup only
migrates up.

修改 schema 后，在仓库根目录运行 `make proto` 重新生成 Go 绑定。它要求
protoc 版本与 CI 一致（`make -s print-PROTOC_VERSION`，即 libprotoc 3.21.12），
并按 go.mod 中锁定的版本构建 protoc-gen-go，保证生成文件头与提交内容一致。
CI 的 `make proto-check` 会重新生成并在与提交的 `server.pb.go` 不一致时失败。
