# ADR-001: 演进为模块化单体架构

- 状态：已接受
- 日期：2026-07-24
- 相关：`docs/design/v2-order-checkout-design.md`（订单 outbox 与对账基础）、`internal/arch`（边界强制测试）

## 背景

当前代码库分层清晰（handler → logic → repository），数据层已按域拆分为 23 个 repo 接口，
订单域具备 outbox 事件与定时对账，异步链路统一走 asynq。但存在三个阻碍未来微服务拆分的结构性问题：

1. **`svc.ServiceContext` 是全局上帝对象**：logic 层 477 处引用、queue/scheduler 41 处引用，
   每个 logic 都能拿到全部依赖，模块边界无从谈起。
2. **`repository.Store` 门面 + `InTx(fn(Store))`**：任何 logic 均可在单个事务内跨任意域读写，
   `DB()` 逃生口直接暴露 `*gorm.DB`。跨域事务是拆分时最难解开的耦合。
3. **代码按访问面（admin/public/auth/…）组织，不按业务域**：同一个域的业务规则散落在
   `logic/admin/<域>`、`logic/public/<域>`、`queue/logic`、`scheduler` 多处。

## 决策

将系统重构为**模块化单体**：按限界上下文划分模块，模块间只允许通过**门面接口（同步）**与
**集成事件（异步）**交互，数据表有唯一属主模块。目标是让"拆出一个微服务"退化为纯机械操作：
门面实现换成 gRPC client、进程内事件换成消息队列、搬表——业务代码不再改动。

### 模块划分

| 模块 | 职责 | 现有资产（repo / 包） |
|---|---|---|
| `identity` | 用户、认证、OAuth、设备、验证码 | UserRepo、AuthRepo、UserAuthRepo、UserDeviceRepo、模块内的 OAuth 适配器 |
| `billing` | 订单、支付、优惠券、余额与提现 | OrderRepo、OrderEventRepo、PaymentRepo、CouponRepo、UserWithdrawalRepo、模块内的订单上下文、订单事件与支付适配器 |
| `subscription` | 套餐、用户订阅、配额、库存、用量入账、客户端应用与模板渲染 | SubscribeRepo、UserSubscriptionRepo、SubscriptionTrafficRepo、EntitlementRepo、ClientRepo、模块内的 inventory、trafficusage 与 render |
| `network` | 节点、流量、edge | NodeRepo、TrafficRepo、`internal/module/network/internal/edgeauth`、`internal/module/network/internal/trafficagg` |
| `support` | 工单、公告、文档、广告、营销 | TicketRepo、AnnouncementRepo、DocumentRepo、AdsRepo |
| `notification` | email / sms / telegram / 站内通知 | TelegramTopicRepo、`internal/infra/mail`、`internal/infra/sms`、`internal/transport/task/email` 与 `sms`、telegram bot |
| `platform`（共享内核） | 配置、系统设置、日志、GeoIP、缓存、ID 生成 | SystemRepo、LogRepo、TaskRepo、InboxRepo、OutboxRepo、`pkg/*` 基础库 |

划分原则：粒度对齐"未来的微服务候选"。`platform` 是共享内核，任何模块可依赖它，它不依赖任何模块。

### 模块结构与交互规则

```
internal/module/<name>/
├── <name>.go        # 门面：接口 + 构造函数 New(deps)
├── contract/        # 本模块拥有的 Command / Query / Result 与跨域只读快照
├── events/          # 集成事件载荷（按需；主题常量由门面导出，其他模块可订阅）
├── entity/          # 模块拥有的持久化数据定义
├── transport/http/  # 本模块的 Hertz handler（只依赖本模块门面与 contract）
└── internal/        # 实现：service / repo / 供应商适配器 —— Go 编译器保证外部不可 import
```

1. **门面与契约**：模块根包提供接口、构造函数与委托内部实现的业务入口；业务 DTO 位于模块自己的
   `contract/`。跨域嵌套数据使用属主模块自己的只读 JSON 快照，不让 contract 形成循环依赖。
   admin/public handler 位于模块的 `transport/http/`，是调用同一模块 service 的薄壳；访问面差异
   （权限、字段裁剪）留在 handler。顶层 `internal/transport/http/routes` 只负责 URL、中间件与 handler 组合。
2. **跨模块写入**：通过属主门面或集成事件协作，每个属主提交自己的事务。异步事件复用
   outbox + 定时发布 + 对账兜底模式，由 `internal/infra/eventbus` 提供投递能力。
   库存预留/回补和流量入账通过属主门面执行，保留各自的 inbox 幂等标记。**禁止新增跨模块事务**。
3. **组装根**：`internal/app.NewApplication` 构造各模块并注入依赖，只有 CLI 导入该根包。
   模块不得反向依赖应用组装；独立的 `internal/app/buildinfo` 仅提供版本元数据。
4. **数据所有权**：每张表唯一属主。跨模块取数走门面调用后内存组装，或事件驱动的冗余字段；
   禁止跨模块 JOIN。

### 边界强制（已落地）

EPay V1/V2 协议实现位于 `billing/internal/payment/epay/protocol`，兼容适配与
业务网关仍在 billing 内部。API 下单的请求标记与可复用支付入口保存在 billing 拥有的
`order.payment_session`；先持久化标记再调用远端，失败后保留未知结果并走现有对账，
不持有跨网络事务锁，也不新增跨模块写入。详见 [EPay 接入](../guide/epay-zh.md)。

- **编译器**：模块实现位于嵌套 `internal/` 下，跨模块 import 内部包直接编译失败。
- **架构测试** `internal/arch`（随 `go test ./...`、CI 与 lefthook pre-commit 运行）：
  - `TestInternalLayout`：固定 internal 的 8 个职责组，限制基础设施的反向依赖；
  - `TestRuntimePackagesStayInternal`：禁止恢复根目录的 queue、scheduler、adapter、initialize Go 包；
  - `TestSharedPackageBoundary`：`pkg` 只收通用库，且不依赖应用代码；
  - `TestAppImportBoundary`：只有 CLI 导入应用组装根；
  - `TestModulePurity`：模块不得 import 应用组装，版本元数据除外；
  - `TestModulesDoNotDependOnFullStore`：模块使用消费者定义的仓储能力，禁止完整 `repository.Store`；
  - `TestModulesUseOnlyTheirOwnRepositories`：模块只使用自己拥有的仓储契约与作用域事务，
    其他模块的数据经属主门面读写（规则 2、4）；契约→属主映射即测试中的 `repositoryOwners`，
    未列出的契约属于共享内核；
  - `TestPlatformDependsOnNoModule`：共享内核不 import 任何业务模块，也不 import 引用全部模块实体的
    `internal/repository`；
  - `TestTaskHandlersOpenNoTransactions`：任务处理器只解码消息并调用模块，事务属于模块；
  - `TestEntryPointsReachModuleDataThroughModules`：CLI、HTTP 中间件、任务处理器与运行时
    bootstrap 不直接读写模块的表；组装根只在属主模块的构造调用里传入属主自己的仓储，其余一律经门面；
  - `TestEventTopicsAreNamedConstants`：集成事件的主题由生产方导出常量（如 `identity.UserRegisteredTopic`），
    订阅与追加 outbox 时不得手写字符串；
  - `TestModuleLayout`：模块只允许暴露门面、`contract/`、`events/`、`entity/` 与 `transport/`；
  - `TestModuleContractsAreIndependent`：禁止中央 DTO 与跨模块 contract 依赖；
  - `TestModuleContractNamesAreUnique`：不同模块不得重复导出同名 contract 类型；
  - `TestModuleTransportOwnership`：handler 只能依赖所属模块门面与 contract；
  - `TestModuleCoreDoesNotImportTransport`：模块核心不得反向导入 `transport/`；
  - `TestHandlerFactories_returnNativeHertzHandlers`：handler 工厂返回 Hertz 原生 handler 类型；
  - `TestLegacyHandlerTreeRemoved`：禁止恢复顶层 `internal/handler`；
  - `TestGoFileNamesAreLowercase`：Go 文件与目录一律使用小写 snake_case 命名。

## 迁移路径

2026-09-05 的目录整理已完成：当前布局见 `docs/design/package-layout.md`。
运行时初始化与热更新已归 `internal/app/bootstrap`，安装页面归 `internal/transport/http/setup`，
迁移引擎和 SQL 归 `internal/app/migration/schema`；SQL 内容和编号保持不变。
本轮依赖收窄后，模块生产代码已清除完整 Store 类型依赖；库存与流量入账改为注入的服务能力。
订单激活编排位于 billing，访客账号创建位于 identity；任务入口只解码并调用 billing。
下文保留此前迁移的历史背景，旧 `ServiceContext` 名称指当时的结构。

每步独立可交付、可上线，不长期分叉：

1. ✅ **立规矩**：本 ADR + `internal/arch` 边界测试（存量豁免、新增即拦截）。
2. **拆 Store**（进行中）：为每个模块定义窄 store 接口（如 billing 只见 Order/OrderEvent/Payment/Coupon），
   `InTx` 收窄为模块作用域；按附录 A.1 逐个把跨域事务改为"本模块事务 + outbox 事件 + 对账"
   （审计 Log 按 A.1 结论豁免为横切关注点）。已落地的机制与改造：
   - ✅ `Store.DB()` 逃生口已移除（零调用者）。
   - ✅ **幂等收件箱**（idempotent-consumer/inbox 模式）：`domain_event_inbox` 表 +
     `repository.InboxRepo`。每个域步骤在自己的事务内插入 `(consumer, event_key)` 标记，
     使 at-least-once 投递与对账重放天然安全；唯一约束同时解决并发竞争（输者回滚）。
     拆分为微服务时各消费方带走自己的 consumer 行，成为其私有 inbox 表。
   - ✅ **领域判定澄清（钱包归 billing）**：`User.Balance / GiftAmount / Commission` 的资金变动
     是 billing 域操作（正文模块表已将"余额与提现"划归 billing），这些列暂住 user 表属数据
     债务，第 5 步拆出独立 wallet 表。因此"锁 user 行 + 扣减余额/赠金/佣金 + 审计日志 + 订单写"
     的事务是**单域（billing）事务**，无需事件化。据此重分类为合规的事务点：
     `renewalLogic`、`resetTrafficLogic`、`purchaseCheckoutLogic`（余额结账）、
     `commissionWithdrawLogic`、`updateUserBasicInfoLogic`（管理员调账）、`trafficStatLogic`
     （network 统计 + 审计）。
   - ✅ **checkSubscriptionLogic（2 处）**：事务收窄为纯 subscription 域写（查询 + 批量置状态），
     邮件通知与用户/服务器缓存失效移到提交后执行（可重试副作用）。
   - ✅ **套餐库存生命周期幂等化**（`internal/module/subscription/internal/inventory/inventory.go`）：库存预留/回补是
     subscription 域写，从 purchase/portal purchase/closeOrder 的 billing 事务中拆出，
     以订单号为键做幂等（`subscription.inventory_reserve/restore`）。下单流程：billing 事务
     建单 → subscription 事务预留（缺货则同步关单补偿，回补因无预留标记而自动跳过）；
     关单流程：billing 事务 CAS 关单 → subscription 事务回补（断点由重试的关单任务经
     status==3 分支续跑）。已知窗口：①建单提交后、预留前进程崩溃且用户在 30 分钟内完成网关
     支付 → 单件超卖（极小概率双重巧合）；②部署切换时刻处于 Pending 的新购订单（旧流程无
     预留标记）关单时不回补 → 建议低峰部署或部署后跑一次库存核对。
   - ✅ **领域窄 store 视图与作用域事务**（`internal/repository/domains.go`）：
     `BillingStore / SubscriptionStore / IdentityStore / NetworkStore` 四个视图 +
     `InBillingTx` 等作用域事务，闭包只拿到本域仓储，跨域写**编译失败**。
     `WalletRepo` 把"钱包归 billing"落到代码（billing 事务经 `Wallet()` 而非 `User()`
     动钱包列）。已迁移的调用点：库存预留/回补、订阅检查、退订两段、流量聚合两段、
     激活的建号/充值/佣金/结算段、关单主事务、portal 补偿。新购主事务已收窄为 InBillingTx。
     **通用 `InTx` 的例外已清零（2026-07-25）**：订阅履约段改用 `subscription_user_serial`
     串行锁（InSubscriptionTx）；注册送试用 ×4 事件化（identity 事务只写本域表 +
     `identity.user_registered` outbox，试用授予由 subscription 消费，见下）；管理员建号/编辑
     两段化（identity 事务先行承载校验，billing 事务随后动钱包，段间失败留下可重调的账号）；
     定时赠金任务分段（每订阅一个 subscription 事务 + 一个 billing 事务，
     `(task, subscription)` inbox 幂等，最后 platform 事务写任务簿记）。业务代码中仅剩
     billing checkout 的自有端口仍以 `InTx` 命名，其事务为单域（billing）。
   - ✅ **退订两段化**（`unsubscribeLogic`）：subscription 事务翻转状态并在取消标记中持久化
     "orderID|应退金额"，billing 事务按标记退款（赠金优先）并写退款标记；两段之间崩溃时，
     用户重试会命中"已扣减但未退款"分支直接续跑退款段。
   - ✅ **流量聚合两段化**（`trafficagg`）：订阅用量计数（subscription）与流量日志（network）
     各自成事务，以 bucket suffix 为幂等键——flush 管线本就重放同一 bucket 直至成功/死信，
     inbox 标记防止已提交的一半被重复计数。
   - ✅ **收件箱保留期**：`InboxRepo.DeleteProcessedBefore` 挂入每日清理任务，与订单事件
     共用 30 天保留契约（所有重放窗口远小于它）。
   - ✅ **首个改造完成：`queue/logic/order/activateOrderLogic`**。原单一跨 4 域事务拆为
     四个单域事务：① identity 访客建号（inbox 存 userId 供重放重绑）→ ② subscription/identity
     履约（开通/续费/重置/充值）→ ③ identity 佣金 → ④ billing 结算（优惠券计数 + Paid→Finished
     CAS + `order.fulfilled` outbox 事件原子提交）。订单在 ④ 之前保持 Paid，
     崩溃由既有 `SchedulerReconcilePaidOrders` 重新驱动，已完成阶段被 inbox 跳过。
     过渡期保留：新购事务内的 user 行锁（按用户串行化配额检查，第 5 步移交 subscription 模块）；
     已知窗口：管理员在履约后、结算前关闭 Paid 订单会留下已履约的 Closed 订单（改造前由行锁互斥），
     补偿属 billing 关单流程的后续工作。
3. **拆 ServiceContext**（已完成）：延续现有 DI 重构，每模块一个 deps 结构；原 `ServiceContext`
   已删除，业务 handler、中间件、route、transport、queue、scheduler 与 initialize 全部改为模块门面或
   任务专属窄依赖。`internal/arch` 的组装根检查（现为 `TestAppImportBoundary`）将 import 组装根的包目录从初始
   71 项收窄到 1 项，仅允许 `cmd` 组装根调用 `app.NewApplication`，新代码不得向业务层回传组装对象。
   管理端可热更新的配置、Telegram 客户端、节点倍率与生命周期回调由 `internal/app/state.State`
   统一持有：配置以不可变快照原子发布，更新过程串行化，避免 HTTP 与队列读配置时和重初始化并发竞争。
   billing 模块（admin order/payment）已按此模式落地：
   激活入队、站点 Host 全部经 Deps 注入，`ActivationEnqueuer` 端口在组装根
   适配 asynq。**结账金流已整体迁入 billing**（`internal/module/billing/internal/checkout`）：
   purchase/renewal/resetTraffic/recharge/preCreate/close 六个流程 + 计价助手，端口化了
   订阅域读取（`PlanReader`/`UserSubscriptionReader`，legacy repo 结构化满足）、订单队列
   （激活 + 延迟关单）、单订阅模式与币种配置；`notify.SettleVerifiedPayment` 的结算逻辑
   （CAS 标记已付 + 激活入队）收编为模块内部函数，close 的网关结算（Stripe/EPay）随迁。
   **portal 门店子域也已迁入 billing**（`internal/module/billing/internal/portal`）：
   访客预下单、支付渠道/余额结账（本就是六边形结构的 `PurchaseCheckoutLogic` 原样收编）、
   订单状态轮询 + 会话兑换（jwt/redis 经端口注入）、
   门店套餐/支付方式列表；`ClientIP` 由 handler 写入请求上下文、模块内解析。
   `v2OrderLogic`（SSE 票据/幂等编排）暂留 legacy 层，四个建单分支、结账、会话兑换
   全部改调 billing 门面；`internal/logic/public/portal` 包已删除，logic 冻结基线的
   `public/order→portal` 边与 svc 基线的 portal 条目一并收缩。
   **支付回调子域也已迁入 billing**（`internal/module/billing/internal/callbacks`）：
   EPay/Stripe/Alipay 回调的验签、订单-支付方式绑定校验、支付期望（金额/币种快照）核对、
   网关复查与结算；结算原语抽为模块内共享的 `internal/settle` 包（checkout 与 callbacks
   共用，防止回调结算与到期结算语义漂移）。`internal/logic/notify` 已删除，svc 基线再收缩。
4. **域优先重组**（✅ 2026-07-24 完成）：把 `logic/admin/<域>` + `logic/public/<域>` 收拢进
   `internal/module/<域>/internal/<子域>`，handler 变薄。实际完成顺序：`support`（耦合最低）
   → `billing`（10 子域：admin order/payment、coupon、userorder、checkout、portal、callbacks、
   settle、v2、wallet）→ `platform`（auditlog、systemsetting、dashboard、publicinfo、tool）
   → `subscription`（plan、storefront、delivery、usersub、selfsub、application）→ `identity`
   （adminuser、profile、authn+oauth+registerpolicy、verifycode、authmethodadmin）→ `network`
   （adminserver、serverapi、edge、nodeconfig）→ `notification`（telegram）。
   **`internal/logic` 目录已删除**：logic 层跨包依赖冻结基线清零后整树消失，7 个模块全部就位。
   跨切面惯例沉淀：运行时可变配置一律经"每请求快照闭包"进模块；进程级副作用
   （Restart/ReinitSubsystem/设备踢线/机器人）经 `internal/app/state.State` 的函数字段或闭包晚绑定；
   验证码原语抽为中立包 `internal/module/identity/internal/verification`；trafficagg 去 svc 化后由 queue 与
   network 模块各自组装。当时仍持 svcCtx 的 queue/handler/initialize/scheduler 已在第 3 步
   改为任务专属窄依赖，`ServiceContext` 随之删除。
5. **数据所有权清算**（✅ 2026-07-24 完成）：表→模块归属定稿如下；
   附录 A.4 的跨模块 JOIN/Preload 全部清理（邮件收件人两段查询、用户统计 Go 侧合并、
   订单计划关联改模块层 PlanReader 填充）；`*userRepo` 物理分家为 identity/
   subscription/billing 三个结构体；**钱包拆分完成**——`user_wallet` 表落地
   （迁移 02143 建表回填、02144 删 user 表三列），Balance/GiftAmount/Commission
   从 user 实体移除，WalletRepo 以 wallet 实体为唯一资金事实源。
   锁序契约：跨双域的流程一律先取 wallet 行锁再取 user 行锁。

   | 模块 | 表 |
   |---|---|
   | identity | `user`、`user_auth_methods`、`user_device`、`user_device_online_record`、`auth_method` |
   | billing | `order`、`order_event`、`payment`、`coupon`、`withdrawals`、`user_wallet` |
   | subscription | `subscribe`、`subscribe_group`、`subscribe_application`、`user_subscribe`、`subscription_entitlement`、`subscription_entitlement_revision`、`subscription_period` |
   | network | `servers`、`nodes`、`server_config_overrides`、`traffic_log` |
   | support | `ticket`、`ticket_follow`、`announcement`、`ads`、`document` |
   | notification | `telegram_topic` |
   | platform（共享内核） | `system`、`system_logs`、`task`、`task_error`、`domain_event_inbox`、`domain_event_outbox` |

   审计日志（`system_logs`）、收件箱（`domain_event_inbox`）与发件箱（`domain_event_outbox`）保持豁免：
   任何域事务可写。表归属与仓储契约的属主一一对应，由 `TestModulesUseOnlyTheirOwnRepositories` 强制。
6. **拆分就绪**：门面换 gRPC 实现（`api/` 已有 protobuf 基建）、~~事件换消息队列~~
   （已完成：asynq 即 broker，见下"队列化改造"——换独立 broker 只动 Publisher 适配器
   与 worker 壳）、搬表/分库（每模块 builder 指向独立连接）、Redis 归属与配置分发。
   前置工作已全部完成：①模块 repo 直连（仓储切割，见下）②queue 编排下沉（激活 saga、
   到期检查、配额任务均已门面化）。
   拆分时每个服务进程建立自己的小组装根：只构造本模块 deps，dto 契约蒸馏为该服务的
   protobuf 定义。

**仓储切割已完成（2026-07-25，伪多进程形态）**：`internal/repository` 收缩为纯契约与
组装包——repo 接口、领域视图、作用域事务、以及由六个模块导出的 builder 拼装出的
`GormStore`（共享连接池）。各模块的 gorm 实现位于 `internal/module/<m>/internal/repo`，
经门面 `NewRepoBuilder` 导出；identity 的跨域缓存级联经 `SubscriptionCacheBridge`
显式注入。实体包按表归属拆分（`entity/usersub`、`entity/wallet`）。模块 import
`internal/repository` 自此为"依赖共享契约"，不再是过渡债务；拆库时每模块把自己的
builder 指向独立连接即可。模块只使用自己拥有的契约，且不允许 import 应用组装（测试强制）。

**事件总线显性化 + 实体入模（2026-07-25）**：
- `internal/infra/eventbus.Bus`：通用 outbox（`domain_event_outbox` 表，迁移 02145）+
  **asynq 作消息队列（2026-07-25 队列化改造）**。生产者在本域事务内
  `Outbox().Append(topic, key, payload)`；发布泵 `scheduler:events:dispatch`（@every 5s）
  驱动 `Bus.Publish`——把未发布事件 enqueue 为 `events:deliver` 任务（TaskID=事件 id 去重，
  冲突即成功，Retention 1h 拓宽去重窗），enqueue 成功即标记已发布，无订阅者的 topic
  直接标记不入队；投递 worker（queue mux）调 `Bus.Deliver` 执行该 topic 全部订阅者，
  首个失败即失败任务，重试/退避/死信归档全归 asynq。at-least-once + 订阅者 inbox 幂等；
  投递并发无序（订阅者以 per-key 串行锁与 inbox 自保）。已发布事件随每日清理按
  30 天保留期删除。换独立 broker（NATS/Kafka）只换 Publisher 适配器与 worker 壳。
- 首个总线事件：`identity.user_registered`（key=userId）。四个注册流仅追加事件；
  subscription 的 trial 子域消费（消费者 `subscription.trial_grant`），禁用策略也消费
  事件（标记记录决策，策略后改不追溯补发）。
- 实体包入模：`internal/model/entity/<pkg>` → `internal/module/<owner>/entity/<pkg>`，
  归属即表属主映射（billing=order/payment/coupon/wallet，subscription=subscribe/usersub，
  identity=user/auth，network=node/traffic，support=ticket/announcement/ads/document，
  platform=system/log/task/client/inbox/outbox）。实体包是纯数据契约，允许跨模块 import；
  `TestModuleLayout` 相应放行 `entity/`。
- **dto 入模（2026-08-21，取代 2026-07-25 的暂缓决策）**：原先 22 个中央 DTO 文件存在
  `user↔common`、`node↔statistics` 等引用环，不能机械地按文件拆包。本次以门面方法的
  业务属主为准蒸馏为各模块 `contract/`；跨域组合响应中的嵌套对象改为属主模块自己的
  只读 JSON 快照。快照允许结构重复但禁止跨模块 contract import，因此既保持 HTTP JSON
  兼容，也消除了中央 DTO 与 import cycle。未来换 gRPC 时，各模块 contract 可独立映射为
  protobuf，不再需要先拆一轮共享 DTO。
- 持久化消费者身份（禁止改名，改名即重放已提交阶段）：`identity.guest_account`、
  `subscription.fulfillment`、`identity.balance_recharge`、`identity.commission`、
  `subscription.trial_grant`、`subscription.quota_grant`、`billing.quota_gift`。

**异步 trace 贯通（2026-07-25）**：asynq 无消息头，`internal/infra/taskqueue` 用 payload 信封携带
W3C trace 上下文——`taskqueue.Client`（`app.Application.Queue` 的类型）在 `EnqueueContext`
时把调用方 span 上下文包进 `{__trace_carrier__, __trace_body__}` 信封；worker 侧
`mux.Use(taskqueue.Middleware())` 解包、以生产者为父开 consumer span（含 task id/重试次数
属性、错误记账），handler 拿到原始 payload。无信封的 payload（老在途任务、无 trace
生产者、调度 tick）直通并得到根 span——**每次任务执行都有 trace id 进日志**。
领域事件更进一步：outbox 行存产生请求的 trace 上下文（`trace_carrier` 列，迁移 02146），
发布泵入队 `events:deliver` 时以**源头请求**（而非泵）为父包装，注册→试用授予全链一条
trace。约束：task 选项必须传给 `EnqueueContext` 而非内嵌 `NewTask`（包装会重建 task，
内嵌选项丢失；存量 6 处已提升）。滚动部署窗口：新生产者+旧 worker 读不懂信封——
单二进制同批升级即可，多副本滚动时先升 worker。

**越权 SQL 清零（2026-07-25）**：模块 repo 中残存的 4 处跨域表访问全部桥接化，
物理拆库自此无 SQL 级障碍：①identity 的邮件收件人 scope 过滤与②admin 用户列表的
订阅条件（原 EXISTS 子查询）→ `SubscriptionScopeBridge.SubscriptionUserIDs`（订阅侧
解析出 user_id 列表，identity 侧 `IN`/空列表 `1=0`）；③identity 的用户统计订单计数
→ `OrderStatsBridge`（billing 属主执行，Go 内 merge）；④subscription 的套餐缓存
失效键（原直查 node 表）→ `NodeCacheKeyBridge`（network 属主解析）。桥在
`repository/builders.go` 声明、属主 bundle 提供、`newGormStore` 装配（network→
subscription→billing→identity 的构建顺序）；identity 的三座桥收拢为 `IdentityBridges`。
方言日期分桶助手抽为 `pkg/orm.DateBucketExpr`。顺带修了 token/uuid OR 条件与其他
过滤器 AND 组合时的优先级缺陷（补括号）。

**业务 DTO 与 handler 入模（2026-08-21）**：`internal/model/dto` 的 22 个中央 DTO 文件已按
`identity`、`billing`、`subscription`、`network`、`support`、`platform` 的所有权迁入各模块
`contract/`；跨域嵌套响应改为模块自有只读快照。原 `internal/handler` 的 HTTP 适配器按实际调用
的模块门面迁入 `transport/http/`，混合的 admin/public user 与 common handler 已按函数拆分，
路由数量、URL、中间件顺序与 HTTP/Swagger 契约保持不变。contract 的 Go 包名继续使用 `dto`，
只为兼容既有 Swagger schema 标识；跨域副本必须使用属主限定的 Go 名称（例如
`BillingSubscribeSnapshot`），并用 Swaggo `@name` 固定原 `dto.*` 文档名。架构测试禁止不同模块
重复导出同名 contract 类型，也禁止模块核心反向导入 `transport/`；route golden 记录具体模块
transport 子包，不再归一化成已删除的 `internal/handler`。

**拆库时的共享表落位（设计预记，第 6 步执行）**：`domain_event_inbox`/`domain_event_outbox`
必须与本域事务同库提交——拆库时**每服务自带一份**（同构表），不共享；`system_logs` 每服务自带
日志表（或改日志事件流）；迁移流按表归属切分历史，新迁移建议带域标记。

**错误码按域分段（2026-07-25）**：存量 66 码**冻结原值**（客户端按数值分支，重编号即
breaking change），新码必须落在属主模块的万段内：Shared=10xxxx、identity=11xxxx、
billing=12xxxx、subscription=13xxxx、network=14xxxx、support=15xxxx、platform=16xxxx、
notification=17xxxx（`pkg/xerr/err_code.go` 的 Band* 常量）。
`TestErrorCodeSegmentation`（AST 解析）强制：值唯一、冻结集不增不减、新码必须入段且
必须有 message；4 个历史无 message 的码（20010/61005/90002/90009）单列冻结，只许收窄。
第 6 步 gRPC 化时业务码经 status detail 过线，分段保证多服务独立演进不撞号。

**组装根现状**（第 3 步收官判定）：原 `svc.ServiceContext` 与它的导入基线均已删除。
`internal/app.NewApplication` 构造基础设施连接、七个模块门面与事件总线，`internal/app/state.State`
持有运行时晚绑定；只有 `cmd` 导入 `internal/app`（`TestAppImportBoundary`）。HTTP、任务与调度入口
只拿到各自的窄依赖结构，业务逻辑对组装根的依赖为零。

## 门面接口草案（示意）

```go
// internal/module/billing/billing.go
package billing

type Service interface {
    Checkout(ctx context.Context, req CheckoutRequest) (CheckoutResult, error)
    CloseOrder(ctx context.Context, orderNo string, reason CloseReason) error
    QueryOrder(ctx context.Context, orderNo string) (Order, error)
    // 供 identity/subscription 查询，替代跨域 JOIN：
    UserPaidOrderCount(ctx context.Context, userID int64) (int64, error)
}

func New(deps Deps) Service { ... } // 由 internal/app 组装根调用

// internal/module/billing/events/events.go
package events

type OrderPaid struct {
    OrderNo     string
    UserID      int64
    SubscribeID int64
    Amount      int64
    PaidAt      time.Time
}
```

订阅方（如 subscription 模块开通订阅、notification 发送通知）在组装根（`internal/app/events.go`）
经 `internal/infra/eventbus` 注册 handler，主题引用属主门面导出的常量；投递语义为 at-least-once，
处理方必须幂等（复用 subscription 库存流程的 inbox 幂等键模式）。

## 风险与对策

- **事务语义变更**：跨域"一个大事务"改为"事务 + 事件"后是最终一致。对策：每类事件配
  对账任务兜底（已有 `SchedulerReconcilePaidOrders` 模式可复制）；先迁读路径、后迁写路径。
- **边界腐化**：靠机器强制（编译器 + arch 测试），基线只减不增；新增基线条目需修订本 ADR。
- **过渡期双轨**（已结束）：`internal/logic` 与 `internal/svc` 已删除，`TestInternalLayout`
  固定 internal 的一级目录，遗留树无法恢复。

## 附录 A：跨域耦合盘点（2026-07-24 快照）

模块映射同上表。注意：`Store` 的 `UserAuth/UserSubscription/UserDevice/UserWithdrawal/SubscriptionTraffic/UserCache`
六个访问器在实现层返回同一个 `*userRepo`（`internal/repository/store.go:135-143`），拆分时
identity 与 subscription 的 repo 实现需要先物理分家。

### A.1 跨域事务点（第 2 步的改造清单）

单域事务 23 处（identity 8、subscription 7、network 3、billing 2、platform 3），无需改造。
**跨 2+ 模块的事务 17 处——已全部处理**（2026-07-24）：4 处随 activateOrderLogic 事件化、
6 处经"钱包归 billing"澄清重分类为单域、2 处（checkSubscription）副作用外移、
2 处（purchase/portal purchase）库存生命周期拆出、1 处（closeOrder）回补拆出、
1 处（unsubscribe）两段化、1 处（trafficagg）两段化。原始清单留档：

| 调用点 | 跨越模块 | 业务流 |
|---|---|---|
| `internal/logic/public/order/purchaseLogic.go:216` | billing+subscription+identity+platform | 下单购买：扣余额、建订单、开通订阅 |
| `internal/logic/public/order/renewalLogic.go:166` | billing+identity+platform | 续费 |
| `internal/logic/public/order/closeOrderLogic.go:94` | billing+subscription+identity+platform | 关单并退回余额 |
| `internal/logic/public/order/resetTrafficLogic.go:97` | billing+identity+platform | 购买式流量重置 |
| `internal/logic/public/portal/purchaseLogic.go:155` | billing+subscription | portal 预下单 |
| `internal/logic/public/portal/purchaseCheckoutLogic.go:649` | billing+identity+platform | 余额支付结账（经 `CheckoutTransaction` 端口） |
| `internal/logic/public/user/commissionWithdrawLogic.go:45` | billing+identity+platform | 佣金提现 |
| `internal/logic/public/user/unsubscribeLogic.go:73` | billing+subscription+identity+platform | 退订并退款 |
| `internal/logic/admin/user/updateUserBasicInfoLogic.go:37` | identity+platform | 管理员改资料（仅审计 Log 跨域） |
| ~~`queue/logic/order/activateOrderLogic.go`~~ | ~~billing+subscription+identity+platform~~ | ✅ 已拆为四个单域事务 + 幂等收件箱（见第 2 步），旧版非事务路径已删除 |
| `queue/logic/subscription/checkSubscriptionLogic.go:31,71` | subscription+identity | 订阅检查 + 清用户缓存（仅缓存失效跨域） |
| `queue/logic/traffic/trafficStatLogic.go:33` | network+platform | 流量统计 + 审计 |
| `internal/module/network/internal/trafficagg/aggregator.go:445` | network+subscription | 流量聚合写回订阅用量 |

由此得出两条改造策略：

- **审计 Log 是横切关注点**，出现在 17 处中的 11 处。不值得为它引入事件：建议将审计日志
  归入 `platform` 共享内核并明确"允许任何模块在自己事务内写审计表"（追加写、无读依赖，
  拆库时改为异步即可），跨域事务清单立减一半。
- **真正的硬耦合是 billing↔subscription↔identity 的资金/开通链路**（购买、激活、退订、结账），
  集中在 6 个业务流。改造顺序建议：先 `activateOrderLogic`（已在异步侧，天然适合事件化），
  再 checkout/purchase（有幂等键与对账兜底），最后退订/提现。

### A.2 `Store.DB()` 逃生口

**零外部调用者**。全仓 `.DB()` 命中均为 GORM 自身 `*gorm.DB.DB()`（连接池设置/ping：
`initialize/config.go:161,250`、`pkg/orm/mysql.go:133,160`）。可在第 2 步直接从 `Store`
接口移除，防止后续被用起来。

### A.3 logic → repo 访问矩阵（横跨 4+ 模块的重灾区）

- `internal/logic/public/order` → billing(Order/Payment/Coupon) + subscription(Subscribe/UserSubscription) + identity(User) + platform(Log)
- `internal/logic/admin/user` → identity(User/UserAuth/UserDevice/UserCache) + subscription(UserSubscription/Subscribe) + network(TrafficLog) + platform(Log)
- `queue/logic` → billing + identity + subscription + network + platform（几乎全部）
- `internal/logic/admin/order` → 仅 billing（干净）

### A.4 跨模块 JOIN / Preload（第 5 步的改造清单）

- `internal/repository/order.go:218,425,435`：OrderRepo(billing) `Preload("Subscribe")` → subscription 表
- `internal/repository/user.go:494-500`：UserRepo(identity) `JOIN user_subscribe` → subscription 表（邮件收件人筛选）
- `internal/repository/user.go:645`：UserRepo 同时 Preload Subscribe(subscription) 与 User(identity)
- `internal/repository/user.go:692-693,733-734`：UserRepo(identity) LEFT JOIN 基于 order 表(billing)的子查询（新单/续费统计）
- `internal/repository/subscribe.go:108`：同域但绕过 repo 直接 `Table("user_subscribe")` 裸表查询

### A.5 logic 层 import 现状

- logic 内部跨包 import：17 处、8 条边，已冻结为 `internal/arch` 基线（`common` ×7、
  `auth/registerpolicy` ×4、`nodeconfig` ×3、`telegram` ×1、`notify` ×1、`public/portal` ×1）。
  其中 `common` 与 `registerpolicy` 属共享内核候选，迁移时移入 `platform`/`identity` 门面。
- logic 之外的调用方：handler 各子包 →对应 logic（常规布线，模块化后改调门面）；
  **`queue/logic/order` → `internal/logic/public/order`（2 处）与 `internal/logic/telegram`（1 处）**、
  `initialize/telegram.go` → `internal/logic/telegram`（1 处）——这 4 处是队列/初始化直接复用
  域逻辑，billing/notification 模块成型时随域收拢，无需单独冻结。
