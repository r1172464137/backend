# 包结构与职责归属

## internal 导航

`internal` 的一级目录按职责收敛为 8 个。目录分组不要求合并为同一个 Go 包；
供应商实现、安全边界和业务模块内部的隔离继续由独立包承担。

| 目录 | 职责与入口 |
|---|---|
| `app` | `NewServices` 是唯一组装入口：`NewApplication` 连接基础设施并组装七个模块，`services.go` 拼出 HTTP、任务与调度服务的依赖；`server.go` 管理启动与重启；`bootstrap` 加载及重载运行时配置；`state` 发布快照，`lifecycle` 管理停止钩子与启动就绪信号（`lifecycle.Readiness`，任务消费者等到它才开始消费任务，最长等待 10 分钟），`buildinfo` 保存版本元数据；`scheduler` 管理定时调度，`migration` 管理迁移 |
| `config` | 配置结构（启动配置 `Boot` 与运行时配置 `Runtime` 两部分）、系统设置转换、数据库和 Redis 配置解析 |
| `auth` | 跨模块的令牌、设备签名与会话、标识规范化、密码、挑战和限流 |
| `module` | 七个业务模块，各自拥有门面、契约、实体、内部实现与业务 handler |
| `transport` | `http/server`、`http/routes`、`http/middleware`、`http/validation`；首次安装页面位于 `http/setup`；设备 WebSocket 位于 `devicesocket`；`task` 统一任务消费服务、路由与各类处理器，邮件批次执行器并入 `task/email` |
| `infra` | 邮件、短信、队列、事件总线、GeoIP 数据库、字段映射、协议编码、请求上下文键与供应商元数据 |
| `repository` | 现有仓储契约、作用域事务和 GORM 组装；各业务仓储实现仍在所属模块内部 |
| `arch` | 目录归属、模块隔离、共享包及组装根依赖方向的测试 |

应用组装按业务拆成 `app/billing.go`、`identity.go`、`subscription.go` 等文件，服务依赖集中在 `services.go`。
只有 CLI 可以导入 `internal/app` 根包，它只负责配置文件的引导与加载，然后调用 `app.NewServices`；
业务模块通过各自的依赖参数取得能力。`TestAssembledDependenciesAreComplete` 在内存基础设施上组装整张
依赖图，任何未接好的依赖都会让它失败。

配置分两部分：`config.Boot` 来自配置文件，进程运行期间不变（监听地址、连接、密钥）；`config.Runtime`
由系统设置表在启动和管理员修改时覆盖，运行时状态只允许通过 `UpdateRuntime` 修改这一部分。
`AppLocation` 在启动时同时设为进程时区，业务时间、GORM 自动时间戳与数据库会话共用一个时钟。
`app/buildinfo` 是独立的版本元数据包，CLI 与管理端版本接口均可读取。
共享基础设施和认证包不得反向依赖应用组装、接入层或业务实现；领域实体作为数据定义可复用。

## 业务代码归位

- 验证码存储与消费属于 `identity/internal/verification`，验证码用途属于 `identity/entity/auth`。
- Edge 请求鉴权属于 `network/internal/edgeauth`。
- 订单幂等上下文属于 `billing/internal/ordercontext`；临时订单、结账 Token 哈希与订单事件频道属于 `billing/entity/order`。
- 库存预留和回补属于 `subscription/internal/inventory`，billing 使用构造时注入的 `subscription.Inventory`，
  调用 `Reserve/Restore` 只传订单号与套餐 ID。
- 流量汇总、重试和死信处理属于 `network/internal/trafficagg`，节点与任务入口均进入 network 的实现。
  订阅用量入账由 `subscription/internal/trafficusage` 执行，network 再提交自己的流量日志事务。
  两段事务各自保存原有 inbox 标记；后半段失败后重试不会再次扣算订阅用量。
- 原 `constant` 已拆分：版本归 `app/buildinfo`，请求上下文键归 `infra/requestctx`，
  订单数据归 billing，验证码用途归 identity。没有保留通用常量集合包。

本次没有拆分 `repository` 的契约与 GORM 组装；这是独立于目录整理的数据层演进项。

## 模块依赖与订单工作流

- 模块生产代码不再依赖完整的 `repository.Store`。每个用例声明实际需要的仓储能力，
  模块门面组合这些能力；共享仓储仍在应用组装层创建。`repository` 提供五种独立的领域事务能力。
- identity 的注册、设备登录和 OAuth 接口已移除不再使用的套餐、用户订阅和订阅缓存仓储要求。
- `subscription.NewInventory`、`subscription.NewTrafficUsage`、`identity.NewGuestAccounts` 在构造时持有仓储。
  库存、用量入账和访客账号操作只接收业务参数，调用者不再逐次传入 Store。
- billing 的 `ActivatePaidOrder` 负责订单激活顺序；任务处理器只解析消息并调用这一入口。
  访客账号与认证记录由 identity 创建，订单绑定由 billing 完成，订阅履约委托 subscription，
  充值、佣金和最终结算留在 billing。通知上下文仍在最终结算前加载。
- 已提交的访客账号通过原 `identity.guest_account` inbox 标记复用；即使绑定订单失败、旧 Redis
  临时订单随后过期，重试也不会再建号。旧明文密码兼容数据由 identity 转换为密码哈希。
  新建账号若同时缺少密码哈希和旧明文则直接拒绝，避免生成空密码账号。
- `TestModulesDoNotDependOnFullStore` 禁止模块恢复完整 Store 依赖；
  `TestModulesUseOnlyTheirOwnRepositories` 禁止模块使用其他模块的仓储契约或作用域事务；
  `TestTaskHandlersOpenNoTransactions` 防止任务处理器重新承担任何事务；
  `TestEntryPointsReachModuleDataThroughModules` 要求入口层与组装根经门面访问模块数据。

共享数据库、共享 ORM 实体以及其他维护任务的存储适配仍存在；它们属于后续数据与任务边界演进。

## 根目录运行时代码归位

- 原 `queue/queue.go` 与 `queue/handler` 合并到 `internal/transport/task`，
  各任务处理器直接位于 `task/email`、`order`、`traffic`、`events`、`sms`、`subscription`、`maintenance`。
  任务消费服务先等待 `lifecycle.Readiness` 的启动就绪信号再开始消费；其 `Stop` 通过 asynq 的 `Shutdown`
  停止，等待正在运行的处理器结束（最长 8 秒），然后关闭队列连接。
- 原 `queue/types` 的任务名称、消息字段和任务 ID 算法并入 `internal/infra/taskqueue`。
  业务生产者依赖这些共享消息定义，不依赖任务消费者。
- 原 `scheduler` 归 `internal/app/scheduler`，负责定时注册任务，调度表达式、重试配置和时区策略保持一致。
- 原 `adapter` 归 `internal/module/subscription/internal/render`，为订阅下发与模板预览构建客户端配置数据。
  最终输出继续由订阅模板和配置的输出格式决定。
- `internal/arch` 禁止恢复 queue、scheduler、adapter、initialize 根目录 Go 包，并禁止模块核心依赖任务或 HTTP 接入层。

## 初始化与热更新

原 `initialize` 按调用职责拆为三处：

- `internal/app/bootstrap`：启动和后台配置热更新。`bootstrap.go` 集中依赖、启动顺序、重载分发与启动迁移；
  `settings.go` 合并站点、注册、邀请、订阅和校验设置；`auth_methods.go` 合并邮件、手机和设备配置；
  `node.go` 合并节点密钥与节点配置；`currency.go`、`telegram.go` 保留各自的运行时副作用。
- `internal/transport/http/setup`：首次安装 HTTP 服务及内嵌页面，保留 `/init`、`/init/config`、
  `/init/database/test`、兼容的 `/init/mysql/test` 和 `/init/redis/test`。
- `internal/app/migration/schema`：数据库迁移引擎、首次安装管理员种子及两种方言的 SQL；
  同级的 `mysql2postgres` 继续负责数据迁移工具。

启动顺序仍为迁移、站点、节点密钥、节点配置、邮件、设备、邀请、校验、订阅、注册、手机、汇率、Telegram。
Telegram 认证方式的配置缺失或无法解码时只记录日志并跳过，不会中止启动。
`bootstrap.Reload` 仅重载所选配置项，不运行启动迁移或节点密钥初始化。
SQL 文件移动时保留原有文件名与内容；分支迁移检查兼容基准提交中的旧目录，目录移动不视为新增迁移。
空的 `mysql.go` 和无调用的自定义 SQL 执行工具已移除。

## pkg 边界

`pkg` 保留 12 个一级目录：`cache`、`conf`、`httpx`、`logger`、`orm`、`random`、
`requestmeta`、`slicesx`、`templatex`、`timeutil`、`trace`、`xerr`。
这些包不得反向依赖应用或业务模块；`internal/arch` 检查这一边界。

## 实际合并

- HTTP 参数绑定与响应包装统一到 `pkg/httpx`，保留原有 JSON 与 Swagger schema 名称。
- AES 加解密内聚到 `internal/auth/deviceauth`，签名与密文格式不变。
- 邮件与唯一的 SMTP 实现统一到 `internal/infra/mail`。
- 服务组、停机钩子统一到 `internal/app/lifecycle`，保留停止顺序、只执行一次及 panic 恢复行为；
  启动就绪信号 `lifecycle.Readiness` 也在这里，任务消费者等到它才开始消费任务，最长等待 10 分钟。
- trace ID/Span ID 直接使用 OpenTelemetry SDK，移除重复的上下文包，避免日志循环依赖。
- 邮箱、手机号与标识规范化统一到 `internal/auth/identifier`。
- Telegram webhook 密钥推导并入 notification 模块。

## 应用实现下沉

- 支付协议和汇率适配器归 billing；应用组装与刷新任务通过 billing 的公开入口使用汇率缓存。
- OAuth 供应商和 state 管理归 identity；不同供应商仍使用独立子包。
- IP 查询的网络适配器与节点倍率归 network；共享 MMDB 读取及日志地理信息补全归 `infra/geoip`。
- 队列追踪适配器、邮件和短信归 `infra`，WebSocket 设备连接管理归 `transport/devicesocket`。
- JWT、设备会话、验证码挑战和限流保留独立的认证子边界，不合成一个万能认证包。
- 构建脚本通过 `internal/app/buildinfo` 注入版本、构建时间与发布渠道。

## 原工具集合的拆分

密码哈希归认证；字段映射放在 `internal/infra/mapping`；系统配置转换放在 `internal/config`；
协议签名和兼容编码放在 `internal/infra/protocolkey`；订单号生成归订单实体；金额格式化归支付。
通用切片、日期和 ETag 分别并入 `slicesx`、`timeutil`、`httpx`。

本轮调整不改变数据库结构、HTTP 接口、支付签名、设备加密、邀请码或订阅 Token 算法。
供应商适配器及安全子包的移动并不等于删掉了它们；统计目录数量时，应区分公共包收拢
和实际 Go 包合并。外部 Go 项目若直接导入过被迁移的服务端包，需要调整，不能导入
服务端的 `internal` 实现；HTTP 客户端协议不受这次目录调整影响。
