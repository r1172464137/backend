# API changes, September 2026

The audit fixes of September 2026 changed the HTTP API in the ways listed here, in one place for frontend and
client maintainers. The complete reference stays [`ppanel.json`](../../ppanel.json) (Swagger 2.0); error names
below are the values of the `code` field in the response envelope.

## Endpoints added or changed

| Endpoint | Change |
|---|---|
| `POST /v1/admin/tool/restart` | Was `GET`; a `GET` is no longer routed. |
| `GET /v1/admin/log/admin/list` | New: audit log of administrator actions. |
| `GET /v1/admin/log/payment/unmatched/list` | New: gateway payments that could not settle an order. |
| `GET /healthz`, `GET /readyz` | New, unauthenticated liveness and readiness probes; see [config.md, Health Checks](config.md#5-health-checks). |
| `GET /v1/app/ws/{userid}/{identifier}` | New device WebSocket, authenticated with the Bearer session token. Browser clients are subject to `AllowedOrigins`. |

## Authentication and verification

- **OAuth sign-in**: `POST /v1/auth/oauth/login` and `POST /v1/auth/oauth/login/token` accept an optional `nonce`
  (at most 128 characters). Recommended flow: generate at least 16 random characters per sign-in attempt, keep
  them in session storage, send the same value on both calls, and never call the token endpoint without a locally
  stored nonce. A mismatch is answered with `OAuthStateInvalid`.
- **OAuth redirects**: Apple and Telegram sign-in redirect to the site host (`Site.Host` in the system settings).
  While it is empty, a redirect is only accepted when it stays on the API's own host (the same host, a subdomain
  or a parent domain); otherwise the sign-in is refused. Operators: [config.md, Site Host](config.md#310-site-host-system-settings).
- **Verification codes**: `POST /v1/common/send_code` and `POST /v1/common/send_sms_code` take a new `cf_token`
  (the Cloudflare Turnstile response). It is required for anonymous registration codes when `RegisterVerify` is
  on. Limit: 20 codes per hour per client IP, then `TooManyRequests`.
- **Device login**: `POST /v1/auth/login/device` requires `cf_token` when `LoginVerify` is on, and is throttled
  per identifier (10 per 15 minutes) and per IP (60 per hour).
- **Availability checks**: `GET /v1/auth/check` and `GET /v1/auth/check/telephone` allow 20 requests per minute
  per IP.
- **Password changes and resets**: `PUT /v1/public/user/password` now returns `{"third_party_bindings": [...]}`;
  the responses of `POST /v1/auth/reset` and `POST /v1/auth/reset/telephone` may carry `third_party_bindings` as
  well.

## Account data and privacy

- `GET /v1/public/user/oauth_methods`, `GET /v1/public/user/devices` and `GET /v1/public/user/info` mask phone
  numbers, provider subjects and device identifiers (for example `3f2a********`). Use the record ids, not these
  values, to identify entries.
- Admin `GET` of `verify_config`, `currency_config` and `node_config`: `turnstile_secret`, `access_key` and the
  outbound `password`, `uuid` and `encryption_password` fields return `"********"` when a value is stored.
  Sending the mask back keeps the stored value; any other value replaces it. `node_secret` is unchanged.
- `GET /v1/admin/marketing/email/batch/list`: new `recipient_count`; `recipients` lists at most 20 addresses,
  followed by "… and N more".

## Administration

- `POST /v1/admin/log/setting`: `clear_days` must be at least 7.
- Administrators: demoting, disabling or deleting the last enabled administrator is refused with `InvalidParams`;
  a `referral_percentage` above 100 is refused.
- The Stripe payment method configuration gains `webhook_endpoint_id`.

## Tickets

- `POST /v1/public/ticket/`: `title` is required (at most 255 characters); `description` at most 10000.
- `POST /v1/public/ticket/follow`: `ticket_id` > 0, `content` at most 65535 characters, `type` one of 0, 1, 2.
- Users may post 30 replies per hour. A staff reply to a Closed ticket is refused with 400 until the ticket is
  reopened; a user reply still reopens it.

## Orders and payments

- `POST /v1/admin/order/`: `type` 1–4, `quantity` ≥ 1, `trade_no` must be empty (it is generated), `subscribe_id`
  is required for type 1 and the new `user_subscribe_id` for types 2 and 3. Coupon usage and stock are reserved
  when the order is created.
- Orders expose `user_subscribe_id`. After a balance checkout, `amount` is the part paid from the balance and
  `gift_amount` the gift balance consumed.
- `GET /v1/admin/log/payment/unmatched/list` (above) lists gateway payments that could not be matched to an
  order.

## Subscriptions and nodes

- `GET /v1/subscribe/config`: 429 above 60 fetches per minute per IP; tokens shorter than 8 characters are
  refused; a subscription whose plan was deleted returns a notice configuration instead of a 500.
- Deleting a plan in the admin panel is refused while Pending, Active or Finished-in-term subscriptions reference
  it.
- Client templates no longer receive `$proxy.RealityServerAddr`, `$proxy.RealityServerPort`,
  `$proxy.CertDNSProvider` and `$proxy.CertDNSEnv`; guarded references render nothing.
- `POST /v1/server/online`: subscription ids the node does not serve are dropped; invalid IPs, or more than 10000
  entries, are refused.

## Telegram bot

- Confirmations of destructive commands are per issuer: only the administrator who issued a command can confirm
  it.
- A user with a `NoExpiry` subscription sees "无限期" (no expiry) instead of a date.
