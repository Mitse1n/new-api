# 多租户（组织采购模型）现状调查与设计方案

> 状态：设计草案 v2（待评审）
> 范围：new-api 主仓库（后端 Go + 前端 React）
>
> **商业模型**：平台提供上游渠道、制定模型价格与订阅套餐 → 组织自助注册并在线购买套餐 → 额度进入组织钱包 → 组织管理员为成员设置消费上限 → 成员消费，由组织钱包统一结算。
>
> **v2 相对 v1 的变更**：删除 BYOK（组织自带渠道）与组织自主定价；把「组织采购订阅 + 组织钱包」从 Phase 3 提升为 MVP 核心；成员额度语义确定为共享池上限而非硬划拨。

## 0. 本次评审已确认的产品前提

| # | 决策 | 影响 |
|---|---|---|
| 1 | 成员额度 = **共享池 + 消费上限**（非硬划拨子钱包） | 权威扣减点唯一，仍在组织钱包；成员上限是前置检查，不参与对账 |
| 2 | 平台**同时**服务个人开发者与组织客户 | 保留「个人组织」抽象以统一代码路径 |
| 3 | 组织**自助注册 + 在线支付** | 复用现有 Stripe / Creem / Waffo / 易支付，无需销售开通后台 |
| 4 | **统一价目表 + 统一套餐** | 不做组织级倍率覆盖；组织只能选套餐，不能改价 |

尚未确定的项集中在第七节，未在文档中擅自替你拍定。

---

## 一、现状调查

### 1.1 身份与角色：扁平的全局用户表

`model/user.go:76` 的 `User` 是唯一的身份主体：

- `Username` / `Email` 全局唯一（`gorm:"unique;index"`），不存在命名空间。
- `Role int` 是一个**全局标量**，取值见 `common/constants.go:190-193`：

  | 常量 | 值 | 含义 |
  |---|---|---|
  | `RoleGuestUser` | 0 | 游客 |
  | `RoleCommonUser` | 1 | 普通用户 |
  | `RoleAdminUser` | 10 | 管理员 |
  | `RoleRootUser` | 100 | 超管 |

  角色不带作用域——「管理员」意味着对**整个实例**的管理员，不存在「A 组织的管理员」。
- `Group string`（`varchar(64)`，默认 `default`）是唯一带「归属」意味的字段，但它是**定价/路由维度**，不是隔离边界。
- `InviterId` / `AffCode` 是邀请返利体系，弱关联，不构成从属关系。

全仓检索 `tenant|organization|workspace|team_id|org_id` 只有三处噪声：渠道上游的 OpenAI 组织头（`model/channel.go:27`）、`setting/chat.go` 里的第三方产品名、以及 `model/task.go:397` 一句注释（讲 task_id 历史上不全局唯一、读取必须 fail closed，"避免选中任意租户的行"）。最后这句恰说明作者已意识到租户语义缺失，但用的是 fail-closed 兜底而非租户列。

`model/system_instance.go` 的 `SystemInstance` 是**集群节点心跳**，与租户无关。

### 1.2 权限系统：casbin 已接入，但缺一个作用域维度

`service/authz/` 落在 `authz_roles` + `casbin_rule` 两张表上，casbin model（`service/authz/enforcer.go:20`）是：

```
[request_definition]
r = sub, obj, act
[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act && p.eft == "allow"
```

1. **`obj` 是资源类型，不是资源实例**。`resources_channel.go` 注册的是 `channel` 上的 `read / operate / write / sensitive_write / secret_view`，判定做字符串全等。「能改渠道」是全局 all-or-nothing。
2. **没有 `dom`（domain）维度**。casbin 原生 RBAC-with-domains（`r = sub, dom, obj, act` + `g(r.sub, p.sub, r.dom)`）正是为多租户准备的，当前未用——这是最省力的扩展点。
3. `middleware.RequirePermission`（`middleware/auth.go:228`）只取 `(userID, role, permission)`，请求里的对象 id 完全不参与判定。
4. subject 只有 `user:<id>` 与 `role:<key>` 两类（`service/authz/permission.go`）。

配套设施齐全：`adapter.go`（GORM adapter）、`override.go`（用户级 allow/deny 覆盖）、`StartPolicySync`（多节点轮询重载）。**底座可用，只差一个维度。**

### 1.3 `group`：事实上的「准租户」维度

| 位置 | 作用 |
|---|---|
| `User.Group` | 用户所属定价档 |
| `Token.Group` / `Token.AutoGroups` | 令牌覆盖档位；`auto` 支持按序尝试多个 group |
| `Channel.Group` | 渠道服务于哪些档位（逗号分隔） |
| `Ability{Group, Model, ChannelId}` | 路由索引，联合主键（`model/ability.go:18`） |
| `setting/user_usable_group.go` | 用户可选 group 白名单（全局 map） |
| `ratio_setting.GroupRatio` / `GroupGroupRatio` | 倍率；后者是 `userGroup → usingGroup → ratio` 二维表 |
| `setting.ModelRequestRateLimitGroup` | `group → [总请求数, 成功数]` 限流 |

group 已能做到：**这批 key 只能走这批渠道、按这个倍率计费、受这个限流约束**。这是相当强的运行时隔离，而且在本方案里它继续承担「套餐 → 可用渠道与价格档」的职责，**不需要改造**。

但 group 不是租户：它不影响控制台里谁能看见什么，不承载成员关系，不能被租户自己管理。

### 1.4 计费、订阅与资金来源：能力齐全，但主体全是个人

这一节是本方案改动最集中的地方，现状比 v1 稿描述的更完善：

- **钱包**：`User.Quota int`（**32 位**）+ 可选 `Token.RemainQuota`。
- **额度预留**：`model/quota_reserve.go` 已实现 **Redis Lua 预留 + DB 兜底**（`TryReserveUserQuota` / `TryReserveTokenQuota`），带 cache schema 校验。
- **资金来源抽象**：`service/funding_source.go:15` 定义了 `FundingSource` 接口，已有两个实现——`WalletFunding`（钱包）与 `SubscriptionFunding`（订阅额度）。`service/billing_session.go` 统一编排预扣 / 结算 / 退款 / 差额。**这是组织化最关键的既有抽象**：组织钱包不是新的资金来源，只是同一个来源换了 owner。
- **订阅体系**（`model/subscription.go`）：
  - `SubscriptionPlan:146` —— 套餐**已经是平台定义的**（价格、周期、`TotalAmount` 额度、`QuotaResetPeriod` 重置周期、`UpgradeGroup` 购买后升级档位、`AllowWalletOverflow` 额度耗尽是否回落钱包、`MaxPurchasePerUser`）。这与「我们制定价格和套餐」的诉求天然吻合，**几乎不用改语义，只需扩展受众维度**。
  - `UserSubscription:253` —— 订阅实例，主体是 `UserId`。
  - `SubscriptionOrder:214` —— 订单，主体是 `UserId`。
  - 消费优先级：**订阅额度优先，耗尽后按 `AllowWalletOverflow` 决定是否回落钱包**（`model/subscription.go:879` `UserActiveSubscriptionsAllowWalletOverflow`）。
  - `UpgradeGroup` / `PrevUserGroup` 在购买/到期时改写 **`User.Group`**（`model/subscription.go:445,472,613`）。
  - 周期重置由 `service/subscription_reset_task.go` 后台任务驱动。
- **支付**：epay / Stripe / Creem / Waffo / Waffo-Pancake，webhook 入口在 `router/api-router.go`，回调把额度打给 `UserId`。
- **兑换码** `model/redemption.go`：全局池，任意用户可兑换。
- **充值** `model/topup.go`、邀请返利 `AffQuota`：均为个人维度。

**结论**：订阅采购这条链路的**能力**是完整的，缺的只是**主体**——所有资金流的收款人和记账人都是 `User`，没有可以「统一采购、再对内分配」的聚合主体。

### 1.5 配置系统：全进程单例

`model/option.go` 的 `Option{Key, Value}` 单表，启动时加载进 `setting/*` 的**进程全局变量**（`RWMap` 或 mutex 保护的裸变量），`SyncOptions` 定时轮询刷新。涉及模型定价、group 倍率、限流、SMTP、OAuth 应用、支付密钥、站点品牌等。

**任何配置都是全实例一份。** 好消息是：既然确认了「统一价目表 + 统一套餐」，本方案**不需要**做定价类配置的租户化，配置分层的工作量比 v1 稿大幅缩小。

### 1.6 数据访问边界：用户级有，组织级无

- **用户级隔离扎实**：`GetAllUserTokens(userId,...)`、自助日志、`buildSelfUserData` 等都按 session 的 `id` 过滤。
- **管理端一律全局**：`GetAllChannels` / `SearchChannels`（`controller/channel.go:101,276`）、`GetAllUsers`、`GetAllLogs`、`GetAllTask`、兑换码、`/api/data` 统计等，唯一门槛是 `AdminAuth()`（全仓 29 处 `AdminAuth()`/`RootAuth()`）。
- 日志表 `model/log.go:59` 有 `UserId / Username / TokenId / ChannelId / Group`，**无租户列**；且日志可能落在独立库（`LOG_DB`）甚至 ClickHouse（`model/main.go:390`）。
- 审计 `middleware/audit.go` 对 admin 写操作兜底留痕，同样无租户归属。

### 1.7 前端：单控制台

`web/src/routes/_authenticated/` 下管理页（channels / users / redemption-codes / system-settings / task-plugins / system-info）与用户页（keys / usage-logs / wallet / profile / playground）混在同一棵路由树，靠 role 和后端 `authz.Capabilities` 能力矩阵做显隐。无租户切换器，无租户级视图。

---

## 二、「一个用户 + 多 token」的能力差距

按本次确定的商业模型重新对齐：

| 能力 | 现状能否 mock | 说明 |
|---|---|---|
| 每客户独立密钥 | ✅ | 一客户一 token |
| 每客户额度上限 | ✅ | `Token.RemainQuota` |
| 每客户可用模型 / IP 白名单 | ✅ | `Token.ModelLimits` / `AllowIps` |
| 每客户走指定渠道池与价格档 | ✅ | `Token.Group` + `Ability` + `GroupRatio` |
| 每客户限流 | ✅ | `ModelRequestRateLimitGroup` |
| **组织统一采购套餐** | ❌ | 套餐主体是 `UserSubscription.UserId`；组织无法作为采购与记账主体 |
| **组织管理员给成员分配消费上限** | ❌ | 只有 user / token 两级扁平额度，无成员维度，无「组织池」概念 |
| **组织成员各自登录、各自建 key** | ❌ | 无组织身份；给账号密码等于把全部 key 和账单交出去 |
| **组织级账单与用量聚合** | ⚠️ | 只能按 token 维度看；一客户多 key 无法聚合到「组织」 |
| 组织内多成员 / 邀请 / 角色 | ❌ | 无成员关系 |
| 组织级审计归属 | ❌ | 审计无租户列 |
| 组织生命周期（停用/删除/导出） | ❌ | 删客户 = 跨 ~10 张表手工清理 |
| 密钥泄露爆炸半径控制 | ❌ | token 泄露即该客户全量沦陷，无组织级轮换策略 |

一句话：**mock 出来的是「配额与路由」，缺的是「采购主体 + 组织内治理」。**

---

## 三、产品设计

### 3.1 三条路线的取舍

| | A. group 即租户 | B. Organization + Membership（推荐） | C. 物理隔离（库/schema per tenant） |
|---|---|---|---|
| 改动量 | 小 | 中 | 大 |
| 能否作为采购/记账主体 | ❌ group 是配置项，挂不住钱包与订单 | ✅ | ✅ |
| 组织内成员治理 | ❌ | ✅ | ✅ |
| 定价档与组织解耦 | ❌ 焊死 | ✅ 组织可换档不换身份 | ✅ |
| 适用 | 现状凑合 | **本商业模型主线** | 强合规/独立部署 |

**推荐 B。** A 的致命问题是 group 是一个配置字符串，无法承载钱包余额、订单、成员关系和生命周期——而这四样正是「组织统一采购」的全部内容。C 的成本主要不在数据库而在 `setting/*` 的进程级全局配置，且今天想要 C 的客户直接开独立实例即可。

### 3.2 Organization + Membership

**核心概念**

- **User（保持全局）**：一个人一个登录身份，复用现有密码 / OAuth / Passkey / 2FA / session 体系，不动。若把用户表按租户切开，认证栈要整个重做。
- **Organization**：租户主体，承载**钱包、订阅、成员、用量归属**。
- **OrganizationMember**：`(user_id, org_id, org_role, spend_limit)`，一个用户可属多个组织。
- **OrganizationInvite**：邮箱 / 链接邀请，明文 token 只展示一次，服务端存哈希。
- **个人组织（Personal Org）**：已确认平台继续服务个人开发者，因此每个用户注册时自动建一个只含自己的 `personal` 组织。这样系统里**不存在「没有组织」的资源**，代码里不需要 `org_id IS NULL` 分支——这是让迁移能收敛的关键决定，也让个人用户与组织客户共用同一套计费代码。

**角色分层（两层，互不混淆）**

- 平台层（沿用 `User.Role`）：`root` / `platform_admin` / `user`。管渠道池、模型定价、套餐定义、系统设置、所有组织。
- 组织层（新增，存 `OrganizationMember.Role`）：

  | 组织角色 | 成员 | API Key | 用量/账单 | 采购套餐 | 成员上限 | 所有权 |
  |---|---|---|---|---|---|---|
  | `owner` | 管理 | 全组织 | 查看 | ✅ | 管理 | 转让、停用 |
  | `admin` | 管理 | 全组织 | 查看 | ✅ | 管理 | ❌ |
  | `billing` | ❌ | ❌ | 查看 | ✅ | 查看 | ❌ |
  | `member` | 仅自己 | 仅自己创建的 | 仅自己 | ❌ | 查看自己的 | ❌ |

  最终权限以后端能力矩阵为准，前端显隐不能替代后端授权。

**租户上下文传递**

- 控制台：session 存 `active_org_id`，顶栏组织切换器；请求头 `X-Org-Id` 显式携带，**服务端每次校验成员关系**，不能只信 session 或请求头。
- API（relay 热路径）：**token 创建时绑定 org**，`Token.OrgId` 随 token 缓存一起加载，请求上下文直接取，**不新增任何数据库查询**。

### 3.3 商业模型：套餐由平台定义，组织采购（本方案主线）

这是 v1 稿缺失、本次新增的核心章节。

```
平台（我们）
  ├─ 维护上游渠道池                    → 现有 Channel / Ability，不变
  ├─ 制定模型单价与 group 倍率          → 现有 ratio_setting，不变
  └─ 定义订阅套餐（价格/周期/额度/档位） → 现有 SubscriptionPlan，扩展受众维度
        │
        │ 组织自助注册 → 在线支付（Stripe/Creem/Waffo/易支付）
        ▼
组织（客户）
  ├─ OrganizationSubscription  周期性额度，优先消费
  ├─ Organization.Quota        钱包余额，订阅耗尽后按 AllowWalletOverflow 回落
  └─ Organization.Group        由套餐 UpgradeGroup 决定 → 决定可用渠道与价格档
        │
        │ 组织管理员设置成员消费上限（不划拨额度）
        ▼
成员 → 自建 API Key → 消费，组织钱包统一结算
```

**关键设计点**

1. **套餐语义几乎不用改**。`SubscriptionPlan` 已有价格、周期、`TotalAmount`、`QuotaResetPeriod`、`UpgradeGroup`、`AllowWalletOverflow`，正是「平台制定套餐」需要的字段。只需新增 `Audience`（`personal` / `org` / `both`）区分面向个人还是组织的套餐，以及可选的成员数上限。
2. **消费优先级沿用现状**：组织订阅额度 → 组织钱包（若 `AllowWalletOverflow`）。这条逻辑已在 `FundingSource` 里实现，换 owner 即可。
3. **`UpgradeGroup` 的作用对象从 `User.Group` 改为 `Organization.Group`**。组织成员的有效 group = 组织 group（成员不再各自持有档位），这既符合「统一采购」语义，也让「买了高档套餐 → 全组织都能用高级模型」成立。
4. **订单主体是组织，操作人是用户**：`SubscriptionOrder` 同时记 `OrgId`（谁买的）和 `UserId`（谁点的），后者用于审计。
5. **不做组织级倍率覆盖**（已确认）。组织想要更低单价，只能升级套餐档位，不能改价。这大幅简化了配置分层，也避免「同一模型在不同组织价格不同」带来的对账复杂度。

### 3.4 额度模型：组织钱包 + 成员消费上限

已确认采用**共享池 + 上限**语义：

```
Organization.Quota (int64 钱包)   ← 唯一权威扣减点
Organization 当前订阅额度          ← 优先消费，周期重置
  ├─ 成员 A  上限 20万/周期  (已用 3万)
  ├─ 成员 B  上限 30万/周期  (已用 30万 → 触顶被拒，但不占用组织余额)
  └─ 成员 C  无上限
       └─ Token.RemainQuota       ← 现有 key 级硬顶，保留
```

- **权威扣减仍然只有一次原子操作**，落在组织（订阅额度或钱包），复用 `model/quota_reserve.go` 的 Redis Lua + DB 兜底模式。
- **成员上限与 key 上限是前置检查**（非权威计数器），超限直接拒绝，不参与最终对账。这是选定语义的直接好处：**不需要引入第二个权威扣减点，计费链路的风险面没有变大**。
- 成员已用量按周期统计，来源是 `logs` 的组织+成员维度聚合（见 4.8），不作为账本。
- 个人组织场景下，组织钱包就是今天的 `User.Quota`，行为完全不变。

⚠️ **`User.Quota` 是 `int`（32 位）**。组织钱包是聚合额度，必须 `int64/bigint`，并遵守 `CLAUDE.md` 的计费安全不变量：单请求饱和仍卡在 int32 边界、钱包换算走 `common.WalletQuotaFromDecimalStrict` + `common.MaxWalletQuota`、所有取整走 `common/quota_math.go`，不得引入新的裸 `int()` 转换。

### 3.5 渠道与模型范围：只有平台共享池

**BYOK 已移出范围**（v1 稿的 §3.4 整节删除）。上游渠道由平台提供，因此：

- `Channel` / `Ability` / `abilities` 联合主键 / 渠道缓存 / 渠道选择（`service/channel_select.go`）**零改动**。
- 组织可用的渠道与价格档完全由 `Organization.Group` 决定，沿用现有 group → ability 路由。
- 组织管理员**不接触渠道**，控制台不出现渠道页。渠道管理仍是平台角色专属。

这一项的移除消掉了 v1 稿里风险最高的路由改动，是本次修订最大的减负。

### 3.6 组织设置：只保留非定价项

由于「统一价目表 + 统一套餐」已确认，`Organization.Settings`（JSON 列）只承载**非定价**的白名单项：

| 允许 | 不允许 |
|---|---|
| 组织显示名、Logo | 模型单价、group 倍率、加价率 |
| 通知 Webhook / 告警邮箱 | 套餐价格与内容 |
| 成员默认消费上限 | 可用 group / 档位（由套餐决定） |
| 可用模型的**收窄**子集（只能在套餐授权范围内减，不能扩权） | SMTP、OAuth 应用、支付密钥 |

提供 `setting.Effective(orgID)`：平台默认 ⊕ 组织覆盖，按 `org_id + version` 缓存，跟随现有 options 同步机制失效。**不把整个 `OptionMap` 租户化。**

---

## 四、代码改动

### 4.1 数据模型（`model/`）

新增：

```go
// model/organization.go
type Organization struct {
    Id        int    `json:"id"`
    Name      string `json:"name" gorm:"type:varchar(64);index"`
    Slug      string `json:"slug" gorm:"type:varchar(64);uniqueIndex"`
    OwnerId   int    `json:"owner_id" gorm:"index"`
    Kind      string `json:"kind" gorm:"type:varchar(16);default:'team'"` // personal | team
    Status    int    `json:"status" gorm:"default:1"`
    Group     string `json:"group" gorm:"type:varchar(64);default:'default'"` // 由套餐决定
    Quota     int64  `json:"quota" gorm:"bigint;default:0"`                   // 钱包余额
    UsedQuota int64  `json:"used_quota" gorm:"bigint;default:0"`
    Settings  string `json:"settings" gorm:"type:text"`                       // 非定价白名单
    CreatedAt int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
    DeletedAt gorm.DeletedAt `gorm:"index"`
}

// model/organization_member.go
type OrganizationMember struct {
    Id         int    `json:"id"`
    OrgId      int    `json:"org_id" gorm:"index:idx_org_user,priority:1,unique"`
    UserId     int    `json:"user_id" gorm:"index:idx_org_user,priority:2,unique;index"`
    Role       string `json:"role" gorm:"type:varchar(32);default:'member'"`
    SpendLimit int64  `json:"spend_limit" gorm:"bigint;default:0"` // 0 = 不限；周期性消费上限
    Status     int    `json:"status" gorm:"default:1"`
    CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

// model/organization_invite.go — 邮箱/链接邀请，存 token 哈希 + 过期时间 + 状态
```

加列（全部 nullable + 索引，便于灰度）：

| 表 | 列 | 备注 |
|---|---|---|
| `tokens` | `org_id` | relay 热路径的租户来源，**必须进 token 缓存** |
| `logs` | `org_id` | 独立 log 库与 ClickHouse 建表语句（`model/main.go:390,398`）同步改 |
| `user_subscriptions` | `org_id` | 订阅主体组织化（见 4.6） |
| `subscription_orders` | `org_id` | 订单归属组织，`user_id` 保留为操作人 |
| `topups` | `org_id` | 充值进组织钱包 |
| `tasks` / `midjourneys` | `org_id` | 异步任务归属与回查 |
| `redemptions` | `org_id` | NULL = 平台全局码 |
| `quota_data`（用量统计） | `org_id` + `user_id` | 组织与成员双维度聚合 |
| 审计表 | `org_id` | 归属追溯 |
| `channels` / `abilities` | **不加列** | BYOK 已移出范围 |

`SubscriptionPlan` 扩展：`Audience string`（`personal`/`org`/`both`）、可选 `MaxMembers int`（套餐允许的成员数上限）。

> **SQLite 兼容**：全部走 `ALTER TABLE ... ADD COLUMN`，不用 `ALTER COLUMN`（参考 `model/main.go` 现有迁移写法）。布尔默认值不要用 `gorm:"default:true"`（见 CLAUDE.md 关于 MySQL/PG 布尔默认值导致 `AutoMigrate` 反复 `ALTER TABLE` 的规则）。

### 4.2 授权层（`service/authz/`）——改动最小、收益最大

casbin model 从 `sub, obj, act` 升到 `sub, dom, obj, act`：

```
[request_definition]
r = sub, dom, obj, act
[policy_definition]
p = sub, dom, obj, act, eft
[role_definition]
g = _, _, _
[matchers]
m = g(r.sub, p.sub, r.dom) && (r.dom == p.dom || p.dom == "*") \
    && r.obj == p.obj && r.act == p.act && p.eft == "allow"
```

- `dom` = `org:<id>`；平台级策略用 `dom = "*"`。
- `resolver.go` 的 `Can(userID, systemRole, permission)` → `Can(userID, systemRole, orgID, permission)`；`Capabilities` 同步加 org 参数。
- `registry.go` 资源注册表新增 `Scope` 字段区分 `platform` / `org`，前端据此渲染两套导航。
- 新增 `resources_org.go`：`org.member.{read,write}`、`org.token.{read,write}`、`org.billing.{read,write}`、`org.subscription.purchase`、`org.settings.write`。**不需要** `org.channel.*`（BYOK 已移出）。
- `middleware.RequirePermission` → `RequireOrgPermission`，从上下文取 `org_id`。
- `adapter.go` / `casbin_rule` 表已有 `V0..V5` 六列，**容纳新维度无需改表结构**。
- `seed.go` 扩展为组织角色的默认策略集。

### 4.3 中间件与上下文

- 新增 `middleware/org_context.go`：控制台请求从 `X-Org-Id`（或 session `active_org_id`）解析并校验成员关系，写入 `constant.ContextKeyOrgId` / `ContextKeyOrgRole`。
- `middleware/auth.go:354 TokenAuth`：从 token 缓存直接取 `OrgId` 写上下文（**零额外查询**）。
- `middleware/model-rate-limit.go`：限流 key 由 `group` 扩展为 `org:group`（同组织内共享配额，避免一个成员打满全组织）。
- `middleware/audit.go`：审计记录带 `org_id`。
- `middleware/distributor.go`：**无需改动**（渠道路由仍走 group）。

### 4.4 数据访问：租户作用域（防漏最关键的一环）

retrofit `org_id` 最大的风险是**某个查询忘了加过滤**。建议：

1. `model/` 加统一 scope：

```go
func OrgScope(orgID int) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB { return db.Where("org_id = ?", orgID) }
}
```

2. 定义 `OrganizationScoped` 接口，所有带 `org_id` 的模型实现它。
3. 加**元测试**：反射枚举所有实现 `OrganizationScoped` 的模型，断言其 list/search 函数都经过 scope（或在显式白名单里）。这类测试符合 CLAUDE.md「保护真实契约」的要求，不是凑覆盖率。
4. 平台管理员的跨租户查询走**显式独立函数**（`ListAllTokensAcrossOrgs` 之类），而不是「orgID == 0 就不过滤」的隐式分支——隐式分支是越权漏洞的温床。

### 4.5 计费与资金来源（改动比 v1 稿估计的轻）

得益于 `service/funding_source.go:15` 的 `FundingSource` 抽象，改动是**换 owner**而非重写链路：

- `WalletFunding` 的扣减对象从 `User.Quota` 换成 `Organization.Quota`（个人组织下等价）。
- `SubscriptionFunding` 的订阅查找从 `UserId` 换成 `OrgId`。
- `model/quota_reserve.go` 的 Lua 脚本按 owner 维度泛化，缓存 key 从 `user:<id>` 换为 `org:<id>`，`CacheSchema` 版本号 +1 触发重建。
- `service/billing_session.go` 的编排逻辑（预扣 / 结算 / 退款 / 差额）**结构不变**。
- **新增一处前置检查**：成员周期消费上限。位置在预扣之前，超限返回 403 且不触碰组织余额。
- `UserActiveSubscriptionsAllowWalletOverflow`（`model/subscription.go:879`）改为按组织判定。

**必须逐条走一遍 CLAUDE.md 的计费安全清单**：validation → EstimateBilling/OtherRatios → quota 转换 → 预扣 → 结算/退款。新增的 clamp 点走 `*Checked` 变体并挂到 `relayInfo.QuotaClamp` → `attachQuotaSaturation`。

### 4.6 订阅与支付链路（本次新增的主要工作量）

- `SubscriptionPlan` 加 `Audience` / `MaxMembers`；平台后台的套餐编辑页（`controller/subscription.go:171,308`）加对应字段与校验。
- `UserSubscription` 加 `org_id`；**保留表名**避免重命名迁移风险，但语义泛化为「组织订阅」，注释说明。`UserId` 保留为购买人。
- 购买流程：组织级 checkout 接口，校验调用者在当前组织有 `org.subscription.purchase` 权限，且套餐 `Audience` 匹配。
- **支付 webhook 是关键改动点**：`StripeWebhook` / `CreemWebhook` / `WaffoWebhook` / `WaffoPancakeWebhook` / `EpayNotify` 的回调需要把额度打到**组织**而非用户。订单里必须持久化 `org_id`，回调只信任订单记录里的归属，**不信任回调参数里的任何身份字段**。
- `UpgradeGroup` / `PrevUserGroup` 的作用对象从 `User.Group` 改为 `Organization.Group`（`model/subscription.go:445,472,613`）。
- `service/subscription_reset_task.go` 的周期重置换主体。
- `model/topup.go` 充值进组织钱包。
- 兑换码：`org_id` 为空的全局码由兑换人当前组织收款。

### 4.7 渠道与 relay 热路径：零改动

- `Channel` / `Ability` / `service/channel_select.go` / `middleware/distributor.go` / `relay/**` 全部不改。
- 组织上下文只从已缓存的 `Token.OrgId` 取，不新增查询。
- **这是本次修订最大的减负项**，v1 稿的 `org:<id>` 私有 group 命名空间、"自有优先/共享优先"路由策略全部作废。

### 4.8 日志与统计

- `Log` 加 `org_id` 并建索引（`org_id` 为最左前缀）；ClickHouse 建表 SQL（`model/main.go:398` 附近）同步更新。
- `GetAllLogs` / `SearchAllLogs` / `/api/data/*` 默认按 org 过滤，平台管理员走独立入口。
- `model/usedata.go` 按天用量表加 `org_id`，支撑两类新报表：**组织总用量趋势**与**成员用量排行**（后者也是成员消费上限的数据来源）。

### 4.9 配置分层

- `Organization.Settings` 只放 3.6 的非定价白名单项。
- `setting.Effective(orgID)` 做平台默认 ⊕ 组织覆盖，按 `org_id + version` 缓存。
- 定价类配置（`ratio_setting.*`）**保持全局**，不做租户化。

### 4.10 前端（`web/`）

- 顶栏组织切换器，`active_org_id` 存 store 并注入所有请求头。
- 路由拆分：`_authenticated/org/*`（组织概览、成员、Key、用量、账单与套餐、组织设置）与现有平台管理导航。
- `admin_permissions` 能力矩阵扩展为 `{platform: {...}, org: {...}}`。
- 新增页面：组织列表 / 成员管理与邀请 / **套餐选购与订单** / **组织账单与成员上限** / 组织设置。
- **删除** v1 稿规划的 BYOK 渠道页。
- 所有新文案走 i18n（`t('English key')` + `web/src/i18n/locales/*.json`），按项目规范用 `bun run i18n:sync`。

### 4.11 影响面速览

| 模块 | 改动量 | 风险 | v1 → v2 变化 |
|---|---|---|---|
| `service/authz/` | 中 | 低 | 不变 |
| `model/` 加列 + 新表 | 中 | 中 | 略减（渠道不加列） |
| `middleware/` | 小 | 低 | **降**（distributor 不改） |
| `service/funding_source.go` + `billing_session.go` | 中 | **高** | **降**（换 owner，不重写） |
| 订阅 + 支付 webhook | 中 | **高** | **升**（v1 未覆盖，现为主线） |
| 管理端 controller 加 scope | 大（面广） | **高** | 不变 |
| `relay/` + 渠道选择 | **零** | — | **降**（BYOK 移除） |
| `setting/` 分层 | 小 | 低 | **降**（无定价覆盖） |
| `web/` | 大 | 低 | 平移（去 BYOK 页，加采购页） |

---

## 五、迁移与灰度

**Phase 0 — 影子期（无行为变化）**
建表、加列（全部 nullable），为每个存量用户创建个人组织，回填其 tokens / logs / topups / subscriptions / orders 的 `org_id`。代码不读新列。可回滚。

**Phase 1 — 授权层升维（无行为变化）**
casbin 升到 `sub, dom, obj, act`，存量策略全部写 `dom = "*"`，语义与今天完全一致。上线后验证权限判定无回归。

**Phase 2a — 组织治理（MVP 上半，不碰钱）**
组织上下文、切换器、成员与邀请、组织角色、组织级 Key、组织用量视图。此时组织已可自助管理，但采购与钱包仍是个人语义。

**Phase 2b — 组织采购与钱包（MVP 下半，碰钱）**
组织钱包、组织订阅与 checkout、支付 webhook 改造、成员消费上限、`UpgradeGroup` 作用于组织。**这是唯一需要严格双写验证或停机窗口的阶段**，与 2a 拆开以便独立回滚。

**Phase 3 — 完善**
组织生命周期（停用/转让/删除申请 + 异步清理）、组织设置白名单、平台跨组织后台。

> v1 稿的 Phase 4（BYOK + 组织定价覆盖）**整体作废**。

每个 Phase 都能独立上线和回滚；Phase 0/1 对用户零感知。

---

## 六、风险与必须遵守的项目约束

1. **数据库三库验证是硬要求**。本方案涉及模型、GORM tag、`AutoMigrate`、索引、唯一约束，按 `CLAUDE.md` 必须在真实的 SQLite / MySQL(≥5.7.8) / PostgreSQL(≥9.6) 上验证，覆盖：全新库、由最新发布版创建的库升级、**连续跑两次启动迁移证明幂等**、独立 log 库与 ClickHouse 路径。交付需记录确切版本号、命令与结果。
2. **越权是头号风险**。管理端有 29 处 `AdminAuth()`/`RootAuth()` 入口，逐个补 scope 时漏一个就是跨租户数据泄露。必须靠 4.4 的元测试兜底，不能靠人肉 review。
3. **支付回调归属是新的高危面**。回调必须只信任订单记录里持久化的 `org_id`，任何从回调参数推断归属的写法都可能被用来把额度打进别人的组织。这是 v1 稿完全没有覆盖的风险。
4. **计费不变量不可破坏**。`User.Quota` 的 int32 与组织钱包 int64 混用是溢出高发区。好消息是本次选定的「共享池 + 上限」语义**没有引入第二个权威扣减点**，风险面比硬划拨方案小得多。
5. **relay 热路径不能退化**。租户上下文一律从已缓存的 token 上取，禁止新增同步查询。
6. **不做的事**：不做 BYOK；不做组织级定价覆盖；不拆用户表（认证栈不变）；不做库/schema per tenant；不把整个 `OptionMap` 租户化。

---

## 七、待确认项（未替你决定）

| # | 问题 | 现文档的处理 |
|---|---|---|
| 1 | **套餐计价维度**：纯额度包 / 额度包 + 席位上限 / 纯按席位计价（每席位 × N，用量不限） | 按**额度包**为主线（与现有 `SubscriptionPlan` 一致），`MaxMembers` 作为可选成员数上限。若要纯按席位计价，套餐模型需另议 |
| 2 | **成员消费上限的周期口径**：自然月 / 与订阅周期对齐 / 自定义天数 | 暂按「与组织订阅周期对齐」描述，需确认 |
| 3 | 组织管理员能否在套餐授权范围内**再收窄**可用模型 | 暂按「允许收窄、不允许扩权」写入 3.6 |
| 4 | 个人用户与组织客户的**注册入口**是否分流；个人用户能否升级/合并为组织 | 未设计，需产品定 |
| 5 | 签到奖励 / 邀请返利在组织语境下归**个人**还是**组织钱包** | 未改动，建议保持个人语义以避免多组织争议 |
| 6 | 成员被移除后其 Key 与历史用量归属 | 建议 Key 始终归组织，创建者只用于权限与审计；不随成员删除 |
| 7 | 是否需要**发票 / 合同 / 对公转账**（当前答案是自助在线支付，首期似不需要） | 未设计 |
| 8 | 组织是否需要**自有 SSO/OIDC**（企业客户常见诉求，会触碰"认证栈不动"的前提） | 未设计 |

---

## 八、如果只想做最小可用版本

若目标是「让客户自己注册组织、买套餐、拉成员进来、各自建 key、管理员能看总用量并给成员设上限」，那就是 **Phase 0 + 1 + 2a + 2b** 的完整范围——因为「组织统一采购」本身就是商业模型的地基，无法推迟。

真正可以推迟的是 Phase 3 的组织生命周期与组织设置白名单：首期可以只支持「组织停用」（软删除 + 平台后台处理），把转让、删除申请、异步清理、模型收窄全部放到下一阶段。这样能省掉约 20% 的工作量，且不触碰采购与计费链路。

如果希望进一步降低首期风险，可考虑把 **2b 的支付 webhook 改造**替换为「平台后台手工为组织充值」作为过渡（组织自助下单，我们人工确认到账），把在线支付回调的高危改动推到第二批——但这与「自助注册 + 在线支付」的目标冲突，需产品权衡。
