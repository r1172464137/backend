# API 变更（2026 年 9 月）

2026 年 9 月的审计修复对 HTTP API 做了以下调整，汇总在此供前端与客户端维护者查阅。完整的接口参考仍是
[`ppanel.json`](../../ppanel.json)（Swagger 2.0）；下文的错误名指响应包裹中 `code` 字段的取值。

## 新增或变更的端点

| 端点 | 变更 |
|---|---|
| `POST /v1/admin/tool/restart` | 原为 `GET`；`GET` 不再路由。 |
| `GET /v1/admin/log/admin/list` | 新增：管理员操作审计日志。 |
| `GET /v1/admin/log/payment/unmatched/list` | 新增：无法结算到订单的网关支付记录。 |
| `GET /healthz`、`GET /readyz` | 新增，无需认证的存活与就绪探针，见 [config-zh.md 健康检查](config-zh.md#5-健康检查)。 |
| `GET /v1/app/ws/{userid}/{identifier}` | 新增的设备 WebSocket，使用 Bearer 会话令牌认证。浏览器客户端受 `AllowedOrigins` 约束。 |

## 认证与验证

- **OAuth 登录**：`POST /v1/auth/oauth/login` 与 `POST /v1/auth/oauth/login/token` 接受可选的 `nonce`（最长 128 个字符）。推荐流程：每次登录生成至少
  16 个随机字符，保存在 session storage 中，两次调用发送同一个值，且绝不在本地没有 nonce 时调用 token 端点。不匹配时返回 `OAuthStateInvalid`。
- **OAuth 重定向**：Apple 与 Telegram 登录重定向到站点地址（系统设置中的 `Site.Host`）。它为空时，只有停留在 API 自身主机（同一主机、其子域名或父域名）上的重定向才会被接受，否则登录被拒绝。运维请参见
  [config-zh.md 站点地址](config-zh.md#310-站点地址系统设置)。
- **验证码**：`POST /v1/common/send_code` 与 `POST /v1/common/send_sms_code` 新增 `cf_token`（Cloudflare Turnstile 响应）。开启 `RegisterVerify`
  时，匿名注册验证码必须携带它。每个客户端 IP 每小时最多 20 个验证码，超出返回 `TooManyRequests`。
- **设备登录**：`POST /v1/auth/login/device` 在开启 `LoginVerify` 时必须携带 `cf_token`；按标识符（每 15 分钟 10 次）和按 IP（每小时 60 次）限流。
- **可用性检查**：`GET /v1/auth/check` 与 `GET /v1/auth/check/telephone` 每个 IP 每分钟 20 次。
- **修改与重置密码**：`PUT /v1/public/user/password` 现在返回 `{"third_party_bindings": [...]}`；`POST /v1/auth/reset` 与
  `POST /v1/auth/reset/telephone` 的响应也可能带有 `third_party_bindings`。

## 账户数据与隐私

- `GET /v1/public/user/oauth_methods`、`GET /v1/public/user/devices`、`GET /v1/public/user/info` 对手机号、第三方账号主体（subject）和设备标识符做了脱敏（例如
  `3f2a********`）。请用记录 id 而不是这些值来标识条目。
- 管理端 `GET` `verify_config`、`currency_config`、`node_config`：`turnstile_secret`、`access_key` 以及出站配置的 `password`、`uuid`、`encryption_password`
  在已有值时返回 `"********"`。原样回传掩码即保留原值，传其他值则替换。`node_secret` 不变。
- `GET /v1/admin/marketing/email/batch/list`：新增 `recipient_count`；`recipients` 最多列出 20 个地址，其后为“… and N more”。

## 管理

- `POST /v1/admin/log/setting`：`clear_days` 至少为 7。
- 管理员：降级、禁用或删除最后一个启用的管理员返回 `InvalidParams`；`referral_percentage` 大于 100 被拒绝。
- Stripe 支付方式配置新增 `webhook_endpoint_id`。

## 工单

- `POST /v1/public/ticket/`：`title` 必填（最长 255 个字符），`description` 最长 10000。
- `POST /v1/public/ticket/follow`：`ticket_id` > 0，`content` 最长 65535 个字符，`type` 取 0、1、2 之一。
- 用户每小时最多回复 30 条。工单处于 Closed 状态时，客服回复返回 400，直到工单重新打开；用户回复仍会重新打开工单。

## 订单与支付

- `POST /v1/admin/order/`：`type` 取 1–4，`quantity` ≥ 1，`trade_no` 必须为空（由服务生成），type 1 必须提供 `subscribe_id`，type 2、3 必须提供新增的
  `user_subscribe_id`。创建订单时会预占优惠券次数与库存。
- 订单暴露 `user_subscribe_id`。余额结账后，`amount` 是余额支付的部分，`gift_amount` 是消耗的赠送余额。
- 新增的 `GET /v1/admin/log/payment/unmatched/list`（见上）列出无法匹配到订单的网关支付。

## 订阅与节点

- `GET /v1/subscribe/config`：每个 IP 每分钟超过 60 次返回 429；短于 8 个字符的 token 被拒绝；套餐已删除的订阅返回提示配置而不是 500。
- 仍有 Pending、Active 或在有效期内的 Finished 订阅引用某套餐时，管理端删除该套餐会被拒绝。
- 客户端模板不再提供 `$proxy.RealityServerAddr`、`$proxy.RealityServerPort`、`$proxy.CertDNSProvider`、`$proxy.CertDNSEnv`；带条件保护的引用渲染为空。
- `POST /v1/server/online`：节点不服务的订阅 id 会被丢弃；无效 IP 或超过 10000 条记录被拒绝。

## Telegram 机器人

- 破坏性命令的确认按发起者区分：只有发出命令的管理员才能确认。
- 拥有 `NoExpiry` 订阅的用户看到的是“无限期”，而不是日期。
