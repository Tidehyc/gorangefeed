# GOrangeFeed 修复待办（Fix TODO）

> 基线：`db60c43`（main）· 生成日期：2026-11-06
> 来源：全量代码通读 + 实测。`go build ./...` / `go vet ./...` / `go test -p 1 ./internal/...`（58 包）当前**全绿**，
> 所以下列问题都属于「测试没覆盖到」而非「编译或回归坏了」。
> 推进顺序：P0 安全 → P1 正确性 → P2 一致性与文档。每条完成后勾选，并跑一遍该条的「验收」。

环境提示：本机沙箱 `/tmp` 只有 10MB，跑任何 `go` 命令前先设 `GOTMPDIR=/home/tide/.cache/gotmp`，否则会 `no space left on device`。

```bash
export GOTMPDIR=/home/tide/.cache/gotmp
cd /home/tide/GoProjects/GOrangeFeed && go build ./... && go vet ./... && go test -p 1 -count=1 ./internal/...
```

---

## 总览

| ID | 级别 | 问题 | 主要文件 | 工作量 |
|---|---|---|---|---|
| FIX-01 ✅ | P0 | 管理接口无 RBAC，任何登录用户可下架/封禁 | `internal/http/admin/routes.go` | M |
| FIX-02 ✅ | P0 | 封禁不生效：token 不失效、refresh 可续命 | `service/moderation`、`service/account`、`middleware/auth.go` | M |
| FIX-03 ✅ | P1 | 审核通过后时间线 score 用错量纲（videoID 当 publish_seq） | `service/moderation/service.go:144` | S |
| FIX-04 ✅ | P1 | 消费端先标记后处理，失败即静默丢事件 | `feed/fanout.go:68`、`feed/hot_score_consumer.go:56`、`notification/consumer.go:81` | M |
| FIX-05 ✅ | P1 | 推荐流翻页重复（page 不参与召回） | `service/feed/service.go:824-961` | S |
| FIX-06 ✅ | P2 | UID 维度限流写好了但没接线 | `middleware/ratelimit.go:113` | S |
| FIX-07 ✅ | P2 | 文档与实现漂移 | `README.md`、`docs/*.md`、`scripts/README.md` | S |
| FIX-08 ✅ | P2 | E2E 断言缺口（M9-05 声称的验证点没测） | `test/e2e/moderation_lifecycle_test.go` | S |

进度：**FIX-01 ~ FIX-08 全部完成并有测试覆盖**（见文末「已完成项实录」）。
FIX-04 是最后一个，单独一个 PR 收尾（消费端 ack 语义，改动面最大）。


---

## FIX-01 · P0 · 管理接口缺 RBAC

**现状（证据）**

- `internal/http/admin/routes.go` 整组只挂了 `middleware.JWTAuth`，没有任何角色判断：

  ```go
  adminAuth := middleware.JWTAuth(jwtMgr, isBlacklisted)
  v1.GET("/reviews", adminAuth, h.ListReviews)
  v1.POST("/videos/:id/review", adminAuth, h.ReviewVideo)
  v1.POST("/videos/:id/takedown", adminAuth, h.TakedownVideo)
  v1.POST("/users/:id/ban", adminAuth, h.BanUser)
  ```

- `migrations/000002_create_users_table.up.sql` 的 `users` 表只有 `status ENUM('active','suspended','deleted')`，**没有 role / is_admin 字段**；全仓库 grep `role|rbac|is_admin` 无实现。
- `docs/PROGRESS.md` 的 M9-04 自称「绑定管理员 RBAC 权限校验」。

**影响**：任意注册用户拿到正常 token 后即可调用 `/api/v1/admin/*`，把任意视频下架、把任意账号封禁。这是当前最严重的一条。

**修复要点**

1. 新增迁移 `0000xx_add_role_to_users`：`role VARCHAR(16) NOT NULL DEFAULT 'user'`，加 `KEY idx_role`。
2. 领域层加 `Role` 类型与常量（`user` / `admin`），`User` 实体透传；账号仓储提供 `FindRoleByID`。
3. 新增 `middleware.RequireRole(roleRepo)`：从 context 取 userID 查角色，不匹配返回 403。**不要**把 role 塞进 JWT claims——权限一旦降级，token 未过期就会继续放行。
4. `admin/routes.go` 的 `adminAuth` 改为 `JWTAuth(...)` + `RequireRole(admin)` 串联。
5. 顺手补一个引导手段：`cmd/api` 启动时若 `users` 表无 admin，按配置项 `bootstrap_admin` 提权（或提供 `make promote-admin ID=x` 的 SQL 脚本），避免上线即锁死。

**验收**

- 新增 E2E：普通用户调 `POST /api/v1/admin/users/{id}/ban` → **403**；admin 用户 → **200**。
- 手动：`go test ./test/e2e -run 'Admin' -count=1` 通过；`curl` 普通 token 得到的响应体是 `code != 0` 且 HTTP 403。

**风险**：新增字段的迁移对已有库是 `ALGORITHM=INSTANT` 可接受的加列（MySQL 8），但 `role` 放在表尾可避免重建表。

---

## FIX-02 · P0 · 封禁后 token 依然可用（鉴权未即时失效）

**现状（证据）**

- `internal/service/moderation/service.go:31/42/57` 注入了 `tokenStore domainaccount.TokenStore`，但**整个 `moderation` 包没有任何一处调用它**（`grep -rn tokenStore internal/service/moderation/` 只有定义和赋值）。
- `BanUser`（`service.go:218-242`）只做 `accountRepo.UpdateStatus(..., UserStatusSuspended)` + 写审计日志。
- `middleware/auth.go` 的 `JWTAuth` 只校验签名和 `IsAccessTokenBlacklisted`，**不查用户状态**。
- `service/account/service.go:127 RefreshToken` 只做 session 轮换，**不查用户状态**；`Login`（同文件 `:91`）才查了 `UserStatusActive`。
- `internal/http/admin/handler.go:173` 硬编码 `"token_revoked": true`，与实际行为不符。
- `docs/PROGRESS.md` M9-05 的验收描述里写了「违规账号封禁后鉴权即时失效」——未实现。

**影响**：封禁后已签发的 access token 可继续用满 `AccessTTL`（2h）；更严重的是 refresh token 链不断，攻击者可以持续换新 access token，**封禁等于没做**。

**修复要点**（三层，缺一不可）

1. **封禁时置标记**：`BanUser` 成功后写 Redis `user:banned:{uid}`（TTL 覆盖 AccessTTL + RefreshTTL），并调用 `tokenStore.RevokeSession` 逐个撤销——但当前 `TokenStore` 没有「按用户枚举 session」的能力，需要在 `CreateSession` 时额外维护 `user:sessions:{uid}` 集合（Redis Set，随 session 过期清理），或直接采用下面的第 3 层以 Redis 标记为主，session 撤销作为尽力而为。
2. **刷新时拦**：`service.RefreshToken` 在 `VerifyRefreshToken` 之后查一次用户状态（DB 或状态缓存），非 active 直接 `401` 并 `RevokeSession`。
3. **鉴权时拦**：`JWTAuth` 增加可选的 `IsUserBanned` 检查（Redis，单次 GET，与现有黑名单同一模式）；`SoftJWTAuth` 命中标记时按游客处理而非放行身份。
4. 撤销/解封场景要能清标记（本仓库暂无解封接口，加 `UnbanUser` 时一并清 `user:banned:{uid}`）。
5. `admin/handler.go` 的 `token_revoked` 改为按 `BanUser` 返回的真实结果填充，不要硬编码。

**验收**（必须在 E2E 里断言，不能只测 status）

1. 用户 A 登录拿到 access + refresh；
2. admin 封禁 A；
3. 用旧 access 调 `GET /api/v1/users/me` → **401**；
4. 用 refresh 调 `POST /api/v1/auth/refresh` → **401**；
5. 重新 `POST /api/v1/auth/login` → **403**（已由 `:91` 保证，作为回归项保留）。

**风险**：给 `JWTAuth` 加一次 Redis 读会抬高每请求开销。建议与黑名单合并成一次 pipeline（`MGET` 两个 key），或对 `user:banned` 做进程内 5s 本地缓存——但缓存窗口内封禁不即时，需在文档里写明取舍。

---

## FIX-03 · P1 · 审核通过后时间线的 score 用错量纲

**现状（证据）**

- 正常发布路径用发号器：`service/feed/service.go:709 s.timelineRepo.AppendWithSeq(ctx, videoID, seq)`、`infra/scheduler/scheduler.go:267` 同样传 `publishSeq`。
- 审核通过路径却调的是不带 seq 的版本：`service/moderation/service.go:144 _ = s.timelineRepo.Append(ctx, videoID)`。
- `infra/persistence/feed/timeline_store.go:70-72`：

  ```go
  func (s *timelineStore) Append(ctx context.Context, videoID int64) error {
      return s.AppendWithSeq(ctx, videoID, videoID)
  }
  ```

  即 **score 落成了 `videoID`**，而全局时间线的 score 语义是 `publish_seq`（`redis/sequence.go` 的 `INCR` 发号，与 `users`/`videos` 自增主键完全不同的量纲）。

**影响**：走「先审后发」的视频过审后，会以一个远小于当前 `publish_seq` 的数字插入 ZSet —— 表现为过审后从全局流里「看不见」，或被 72h floor 裁剪逻辑误伤。这一路恰好是审核链路的唯一出口，属于静默错序。

**修复要点**

1. `ReviewVideo` 的 `approve` 分支改为：先 `videoRepo` 取该视频的 `PublishSeq`（`domain/video/entity.go:35` 已有字段），再 `AppendWithSeq(ctx, videoID, publishSeq)`。
2. 若 `PublishSeq == 0`（先审后发的视频在创建时可能未发号），补发号：注入 `domaincreator.SequenceGenerator`，`NextPublishSeq` 后 `videoRepo.UpdatePublishSeq` 再写时间线。
3. 建议顺手把 `Append`（无 seq 版本）标记为 deprecated 或直接删除，避免再被误用——目前生产代码里只剩审核这一处调用。

**验收**

- 单测（mock timelineRepo）：构造 `PublishSeq=5000`、`videoID=7` 的视频，过审后断言 `AppendWithSeq` 收到 `seq=5000`，而不是 7。
- E2E 追加断言：先审后发视频过审后出现在 `GET /api/v1/feed/global` 首页，且其 `publish_seq` 与 ZSet score 一致。

**注**：`AppendTimeline`（`service/feed/service.go:700`）也是 `AppendTimelineWithSeq(videoID, videoID)` 的同款写法，一并检查调用方是否都该传真实 seq。

---

## FIX-04 · P1 · 消费端「先标记后处理」导致静默丢事件

**现状（证据）**

三处消费者都是先落 `consumed_events` 再执行业务：

- `internal/service/feed/fanout.go:68 isNew, err := s.deduplicator.MarkConsumed(ctx, eventID)`
- `internal/service/feed/hot_score_consumer.go:56` 同上
- `internal/service/notification/consumer.go:81` 同上

而 `internal/infra/rabbitmq/deduplicator.go:60` 明明提供了一个事务化封装 `ExecuteWithDeduplication(ctx, eventID, fn)` —— 全仓库**生产代码零调用**，只出现在 `*_test.go` 与 mock 里。

**影响**：业务处理返回 error → 上层 Nack 重投 → 第二次进来 `isNew=false` → 直接 ACK。事件永久丢失：点赞数不加、热度不涨、通知不发。当前能兜住一部分是因为 `notifications` 表有 `UNIQUE(recipient_id, event_id)`，但那条路径的重试根本不会发生。

**修复要点（初版方案，实际落地时的偏差见文末「FIX-04 实录」）**

1. 能进 MySQL 事务的业务（写通知、写计数落库）改为 `ExecuteWithDeduplication`：`MarkConsumed` 与业务写入同事务提交，失败一起回滚。
2. 无法进事务的副作用（Redis `ZINCRBY` 时间线/热榜、SSE 广播）分两类处理：
   - **天然幂等**（`ZAdd`/`ZRem`、`ZINCRBY` 之外的重建型操作）→ 先执行业务，成功后再 `MarkConsumed`；
   - **非幂等**（`ZINCRBY` 累加）→ 用事件 ID 做去重键（如 `hot:applied:{event_id}` SETNX，TTL 覆盖重投窗口），保证重复消费不重复累加。
3. 明确 `consumed_events` 的清理策略：当前无 TTL / 无归档任务，长期会膨胀。
4. 补一条「业务失败」单测：mock 业务返回 error，断言 `consumed_events` 中**没有**该 eventID，且返回的 ack 动作为 Nack。

**验收**

- 新增「业务失败不标记」用例（三处消费者各一条）：`TestFanoutService_FanoutFailure_DoesNotMarkConsumed`、
  `TestConsumer_ProcessingFailure_DoesNotMarkConsumed`、`TestHotScoreConsumer_ApplyFailure_DoesNotMarkConsumed`。
- 手工验证：临时让通知写库抛错，观察日志里同一 `event_id` 最多重投 5 次且最终进 `<queue>.dead`，而不是一次就 ACK。
  这一条**没有做**（需要停 MySQL 并盯 worker 日志的运维演练），改用单测覆盖判定表 + 失败不标记的不变量，见实录。

---

## FIX-05 · P1 · 推荐流翻页返回重复内容

**现状（证据）**

`internal/service/feed/service.go:824 GetRecommendFeed`：

```go
page, seed, err := domainfeed.DecodeRecommendCursor(cursor)   // 825
...
if seed == 0 { seed = time.Now().UnixNano() }                  // 837-839
candidates, _ = s.recallEngine.Recall(ctx, targetRecall, seed)  // 849  ← page 没参与
...
nextCursor = domainfeed.EncodeRecommendCursor(page+1, seed)     // 961  ← page 只用来拼下一页游标
```

`page` 从头到尾只被用来生成 `nextCursor`，召回输入 `(targetRecall, seed)` 不变 ⇒ 第 2 页召回集合与第 1 页完全相同，仅靠曝光过滤（`exposureRepo.FilterExposed`）能掉一部分，游客（`viewerID == 0`）则完全掉不了。

**修复要点**

- 方案 A（最小改动）：把 page 混入 seed —— `recallSeed := seed + int64(page)*prime`，保证「同一 cursor 重复请求结果一致、不同 page 结果不同」两个语义同时成立。
- 方案 B（更彻底）：召回池固定，游标改为 `{seed, offset}`，对召回结果做稳定排序后按 offset 切片。与热榜 `as_of+offset` 的写法保持一致，推荐优先选 B。

实际采用 **方案 B**（见文末「FIX-05 实录」）。方案 A 被否掉的原因：换 seed 会让每一页召回的是**另一套候选**，
池子随页漂移，曝光过滤与打散的输入也跟着变，深翻页仍然可能漏内容；只有「一次召回、一次排序、按 offset 切片」能同时保证两页不重叠且不遗漏。

**验收**

- E2E：连续请求 page1 / page2，断言 `video_id` 交集为空且都非空；重复请求同一 cursor 两次，断言结果完全一致（幂等）。
- 已落地：`E2E-M4.5-07_Recommend_Pagination_No_Duplicates`（整轮翻页零重复、20 页内必然终止）、
  `E2E-M4.5-08_Recommend_Cursor_Idempotent`（同一 cursor 两次结果逐项相同）。

---

## FIX-06 · P2 · UID 维度限流未接线

**现状**：`internal/infra/middleware/ratelimit.go:113` 定义了 `UserRateLimiter`，但全仓库唯一调用点是 `ratelimit_test.go:127`；`internal/infra/router/router.go` 里 grep 不到任何 `RateLimit`。实际只有登录/注册组挂了 IP 限流（`internal/http/account/routes.go:19`，10 次/分）。

**修复要点**：给写路径挂 UID 限流，限额用 NFR-SEC-07 / DESIGN.md 的设计值（点赞、评论、关注 = 30 次/分/UID）；挂在 `JWTAuth` **之后**，这样 key 是用户 ID 而不是 IP。

**范围说明**：原待办里还写了「发布视频、上传、举报」，实际只做了 NFR-SEC-07 明确列出并给了数值的三类——
发布/上传在 NFR 里**没有设计值**，举报接口**根本不存在**。自己编一个限额会变成「文档有、需求无」的新漂移，
所以留空并在此说明，等有明确的产品口径再补。

**验收**：E2E 压同一 UID 的点赞接口，第 31 次返回 429 且 `error.code == "rate_limited"`；另一用户同接口不受影响；
限流 key 的 TTL 落在 (0, 1m]。

---

## FIX-07 · P2 · 文档与实现漂移

| 位置 | 文档原来写的 | 实际 | 改成了 |
|---|---|---|---|
| `README.md:3` | 「采用垂直切片架构（Vertical Slice）」 | 同文件 `:9` 与代码分层都是整洁架构（Clean Architecture），domain 零框架依赖 | 整洁架构（Clean Architecture） |
| `README.md` 领域列表 | 只列 7 个领域 | `internal/domain/` 下 12 个包 | 列出全部 12 个 |
| `docs/REQUIREMENTS.md` 验收标准 1 | 「跑通 `scripts/e2e.sh`」 | 无此脚本，且 Makefile 里也没有 e2e 目标 | 新增 `make e2e`，文档改指它 |
| `docs/REQUIREMENTS.md` 边界 | 收藏夹列为「排除，规划至二期」 | M8 已实现 `internal/domain/asset`（多文件夹 + 收藏关系 + 观看历史/播放进度） | 移到「包含」，并从排除项删掉 |
| `docs/PROGRESS.md:249` | 「支持 10 万+ 敏感词毫秒级匹配」 | `ac_matcher.go:46/52` 是 19 条硬编码种子词（10 高危 + 9 中危）；算法确实是 AC 自动机 | 「内置 19 条种子词库 + 运行时 `AddWord` 扩展」，并去掉「双数组」（实现是带 fail 指针的 trie，不是双数组） |
| `docs/BENCHMARK_REPORT.md:85` | 「后续可引入分布式单调发号器」 | M7 已实现 `internal/infra/redis/sequence.go`（`INCR` + `publish_seq`） | 改成「已落地，下一步可演进为号段模式」 |
| `scripts/README.md` | 声称存在 `seed/`、`deploy/` 两个目录 | 两个目录都不存在 | 改成描述真实的 `check_migrations.sh` 与 Makefile 入口 |

**修复原则**：以代码为准改文档。凡是「性能/规模」类表述，只写能从代码里数出来的东西——
敏感词那条写 19 条种子词、写算法复杂度性质（随文本长度线性、与词库规模无关），不写「10 万+」「毫秒级」这类没有测量支撑的数字。

---

## FIX-08 · P2 · E2E 断言缺口

原状：三处「说验证了、其实没验」的地方。

| 缺口 | 现在 | 落在哪个改动里 |
|---|---|---|
| 封禁用例只断言 `users.status` 与审计日志，没测 token 失效 —— 这正是 FIX-02 能长期潜伏的原因 | 旧 access → 403、refresh → 401、relogin → 403、`token_revoked=true` 全部断言 | FIX-02（PR #1） |
| 推荐流翻页无「两页不重复」断言 | `E2E-M4.5-07` 整轮翻页零重复 + 20 页内终止；`E2E-M4.5-08` 同 cursor 幂等 | FIX-05（PR #2） |
| 审核通过后的时间线写入无断言 | 断言 `timeline:global` 的 score == 视频的 `publish_seq`，且 `publish_seq > 0` 不等于 videoID | 本次（FIX-08） |

---

## 建议的推进顺序

1. ✅ **FIX-01 + FIX-02 一起做**（同属后台权限闭环，共用一份 E2E：普通用户 403 / admin 200 / 封禁后三种 token 全失效）。
2. ✅ **FIX-03**（改动最小、影响面清晰，先落地验证时间线量纲）。
3. ✅ **FIX-05 + FIX-08 的翻页断言**一起。
4. ✅ **FIX-06**（限流接线）。
5. ✅ **FIX-07**（纯文档）。
6. ✅ **FIX-04**（触及三处消费者的 ack 语义，改动风险最高，单独一个 PR，跑全量 `./internal/...` + E2E 回归）。

每个 PR 的验证口径统一为：`go build ./...` + `go vet ./...` + `go test -p 1 -count=1 ./internal/...` + 对应 E2E 用例；文档类改动只跑 `markdown` 检查即可。

---

## 已完成项实录（2026-11-06）

### FIX-03 · 过审后时间线 score 改用 publish_seq

- `internal/service/moderation/service.go`：`ReviewVideo` 的 `approve` 分支不再调 `timelineRepo.Append`
  （其实现是 `AppendWithSeq(videoID, videoID)`），改走新增的 `appendApprovedToTimeline`：
  先用 `videoRepo.FindByID` 取真实 `PublishSeq`，缺失时通过注入的 `SequenceAllocator` 补发号并
  `UpdatePublishSeq` 落库，再 `AppendWithSeq`。拿不到序号时**放弃写入并告警**，绝不退回用 videoID 当 score。
- 构造函数新增 `seqAllocator SequenceAllocator` 参数（本地窄接口，只要求 `NextPublishSeq`），
  `cmd/api/main.go` 传 `seqGen`，E2E 传 `seqGen`，单测传 nil 或 mock。
- 新增单测：`TestModerationService_ApproveAppendsTimelineWithPublishSeq`（断言 score=5000 而非 videoID=301）、
  `TestModerationService_ApproveAllocatesMissingPublishSeq`（断言补发号 + 落库 + 写入）。

### FIX-01 · 管理接口 RBAC

- 迁移 `migrations/000013_add_role_to_users.up.sql`：`users.role VARCHAR(16) NOT NULL DEFAULT 'user'` +
  `idx_role`，单条 `ALTER` 且追加在表尾，MySQL 8 走 INSTANT。已在本机库执行（`schema_migrations` version=13）。
- `domain/account`：新增 `Role` 类型与 `RoleUser`/`RoleAdmin`，`User.Role` 字段；注册时显式写 `RoleUser`，
  避免零值空串落库。
- `middleware.RequireRole(resolver, role)`：按 `userID` 查库判角色，查不到与角色不符都返回 403
  （不区分「用户不存在」与「权限不足」，避免探测）。
- `admin/routes.go`：整组 `v1.Use(JWTAuth, RequireRole(admin))`，所有管理接口（含新增的 unban）统一收口。
- 运维入口：`make promote-admin ID=<user_id>`（首次部署没有管理员时用它提权）。

### FIX-02 · 封禁即时失效

- `domain/account.TokenStore` 扩了三个方法：`MarkUserBanned` / `ClearUserBanned` / `IsUserBanned`，
  Redis 键 `auth:banned:{userID}`，默认 TTL 30 天（`bannedUserDefaultTTL`）。放在 TokenStore 而不是
  另建接口，是因为 access token 不携带 sessionID，无法通过撤销会话间接失效。
- `middleware/auth.go`：`JWTAuth` / `SoftJWTAuth` 的第二个参数由「黑名单函数」改为 `AuthStateChecker`
  （同时提供 `IsAccessTokenBlacklisted` 与 `IsUserBanned`），11 个路由文件的 20 处调用改为直接传 `tokenStore`。
  硬鉴权命中封禁返回 **403**；软鉴权命中则降级为游客，不阻断匿名可读接口。
- `moderation.BanUser` 现在写封禁标记，并返回 `credentialsRevoked bool`；`admin/handler.go` 的
  `token_revoked` 改为回填这个真实结果（此前是硬编码 `true`）。
- 新增 `moderation.UnbanUser` + `POST /api/v1/admin/users/:id/unban`：恢复 `active` 并清标记。
- `account.RefreshToken` 先查用户状态，非 active 直接 401 并撤销该会话 —— 这是原先「封禁可被 refresh 链无限续命」
  的根因。

**验证**（`go test ./test/e2e -run TestM9_ModerationLifecycle_EndToEnd -v`，全部 PASS）：

| 断言 | 实测 |
|---|---|
| 普通用户 `GET /api/v1/admin/reviews` | 403 |
| 管理员同接口 | 200 |
| 封禁后旧 access 调 `GET /api/v1/users/me` | 403 |
| 封禁后 `POST /api/v1/auth/refresh` | 401 |
| 封禁后 `POST /api/v1/auth/login` | 403 |
| 封禁响应 `token_revoked` | true |

单测侧新增：`TestJWTAuth_BannedUserRejected`、`TestSoftJWTAuth_BannedUserTreatedAsGuest`、`TestRequireRole`
（admin 200 / 普通用户 403 / 角色解析失败 403）、`TestAdminHandler_NonAdminForbidden`、
`TestAdminHandler_UnknownRoleOwnerForbidden`、`TestAdminHandler_UnbanUser`。

**遗留取舍（写在这里以免以后被当成 bug）**

1. `isBanned` 查询失败时按「未封禁」处理（fail-open）：Redis 抖动不应该让全站用户 401。
   代价是 Redis 挂掉期间封禁不生效，但 refresh/login 仍会拦住续命。
2. 封禁标记是 Redis 键，若 Redis 数据丢失，已签发的 access token 会在剩余 TTL（≤2h）内复活。
3. 未做「按用户枚举并撤销会话」：`TokenStore` 没有 `user -> sessions` 反向索引，当前靠封禁标记 +
   refresh 查库覆盖，够用且不需要新增索引维护成本。

### FIX-05 · 推荐流翻页重复

**根因**：游标里的 `page` 只用来拼下一个游标（`EncodeRecommendCursor(page+1, seed)`），
从未进入召回或排序，`Recall(targetCount, seed)` 的入参在每一页完全一致
⇒ 每一页都从同一个池子的头部重新取。曝光过滤只能偶然削掉一部分，游客（`viewerID == 0` 不走过滤）则 100% 重复。

**改法**：游标从 `{page, seed}` 换成 `{offset, seed}`，语义从「第几页」变成「已消费了多少条」：

- `internal/domain/feed/entity.go`：`RecommendCursor.Page` → `Offset`，编解码同步改；
  空游标返回 `(0, 0)`，负 offset 视为非法；旧格式游标（只有 `page` 字段）解码为 `offset=0`，
  即**重启会话**——这比在一个新召回的池子里猜位置安全。
- `internal/service/feed/service.go` `GetRecommendFeed`：
  1. 池子大小只由 `limit` 决定（`4×limit`，夹在 40~200），**与游标无关**，否则翻页会换池；
  2. 整个池子用会话内稳定的 `seed` 召回后，**只打散一次**得到 `ordered`；
  3. 从 `ordered[offset:]` 起按窗口向走，窗口内做曝光过滤，取够 `limit` 或走完池子为止；
  4. 推进量按**窗口内实际消费到的位置**算（跳过项也算消费），而不是按「服务了几条」，
     否则下一页会回头重扫被过滤掉的部分；
  5. 池子走完仍不满时保留原有的 timeline / 冷区补填，补填后 `position` 直接置到池尾，
     `has_more=false`——补填项在池子之外，给游标就等于把它们重复发一次。
- 顺序是「先排序、后过滤」，不是「先过滤、后排序」：后者会让池子随用户观看行为收缩，
  第 2 页算出来的 offset 落在已经位移过的序列上，重复会以另一种形态回来。

**测试**

- 新增 `TestService_GetRecommendFeed_PaginationNoDuplicates`：limit=2 连翻整轮，
  断言游客得到 `[1,4,5,6,7,2,3]`、已看 V1 的用户得到 `[4,5,6,7,2,3]`，且 **10 页内必然终止**（不会死循环）。
- 反证：把 `nextCursor` 改回用 `offset`（即游标不推进），该用例立刻失败并报
  `pagination did not terminate after 10 pages, last cursor = ..."offset":0...` —— 与线上症状一致。
- `TestService_GetRecommendFeed_ExposureAndDiversification` 的用户页期望值随之从 `[2,4,5,6]` 改为 `[4,5,6,7]`：
  排序先于过滤，被过滤项只在走窗口时被跳过。这是**有意的语义变化**，不是为了让测试变绿。

**遗留取舍**

- 池子的稳定性依赖召回的确定性：`explore` 通道用 `seed` 播种（稳定），但 `hot` 通道按
  `time.Now().Truncate(time.Minute)` 取快照、`latest` 通道按最新 N 条取——**跨分钟翻页时池子仍会小幅漂移**，
  极端情况下可能漏一条或重一条。彻底解决要把池子按 `seed` 服务端缓存（会话级），代价是多一份 Redis 状态，
  留作后续优化，当前不引入。
- 池子上限 200 条：`limit=30` 时最多服务约 6 页，翻到底 `has_more` 会转 false 而不是无限深挖。
  这是有意的——推荐流不需要无限下滑，深翻页本来就应该回到重新召回（刷新）。

### FIX-06 · UID 维度限流接线

**根因**：`middleware.UserRateLimiter` 早就写好了（含 `uid:` 前缀回落 IP 的逻辑），但只在它自己的单测里被调用过。
`PROGRESS.md` M1-07 写着「按 UID 限制高频操作（30 次/分）」、`DESIGN.md` 也这么写，**实现从来没接上线**。

**改法**：在三个写路径组上挂 `middleware.UserRateLimiter(rdb, <action>, 30, time.Minute)`，
一组一个桶（`like` / `comment` / `follow`），而不是共用一个桶——爱评论的用户不该被堵得点不了赞。

| 文件 | 挂载点 | 桶 |
|---|---|---|
| `internal/http/interaction/routes.go` | `POST/DELETE /videos/:videoId/like`、`POST/DELETE /comments/:commentId/like` | `like` |
| `internal/http/interaction/routes.go` | `POST /videos/:videoId/comments`、`DELETE /comments/:commentId` | `comment` |
| `internal/http/relation/routes.go` | `POST/DELETE /users/:userId/follow` | `follow` |

**顺序很重要**：限流器挂在 `jwtAuth` **之后**。`UserRateLimiter` 的 key 取 `middleware.GetUserID`，
放在鉴权之前拿到的是空值，会退化成按 IP 计——那就不是「UID 维度限流」了。
`rdb == nil` 时 `RateLimiter` 自身直接放行，所以不需要像 `account/routes.go` 那样再加 `if rdb != nil`。

**测试**：`TestM4_InteractionLifecycle_EndToEnd` 新增 Phase 10：

- 用一个**独立注册的探针用户**打 31 次 `POST /videos/:id/like`，断言第 **31** 次才 429（前 30 次都是 200，
  点赞在持久层是 `ON DUPLICATE KEY UPDATE id = id`，重复点赞本身就是幂等的）；
- 断言 429 响应体走统一 `response.Fail`，`error.code == "rate_limited"`；
- 换另一个用户打同一接口，断言**不受影响**（证明 key 是 UID 不是 IP）；
- 断言 Redis 键 `ratelimit:like:uid:{uid}` 的 TTL 落在 `(0, 1m]`。

用独立用户而不是复用 Phase 1 里的 uidA：Phase 1-9 已经消耗了同一分钟内的点赞配额，
复用会让「第 31 次」这个断言变成随机通过。

### FIX-08 · E2E 断言缺口

三处缺口全部补上，各自跟着对应改动落地（这条本身只新增了最后一项）：

| 缺口 | 断言 | 落在哪 |
|---|---|---|
| 封禁只验 `users.status` | 旧 access 调 `/users/me` → 403、`/auth/refresh` → 401、relogin → 403、`token_revoked=true` | FIX-02（PR #1） |
| 推荐翻页无「两页不重复」 | `E2E-M4.5-07` 整轮零重复且 20 页内终止、`E2E-M4.5-08` 同 cursor 幂等 | FIX-05（PR #2） |
| 过审后无时间线断言 | `ZScore("timeline:global", videoID) == video.PublishSeq`，且 `PublishSeq > 0`、不等于 videoID | 本条 |

**为什么这条断言能咬住 bug**：`timelineRepo.Append(videoID)` 真实的实现是 `AppendWithSeq(videoID, videoID)`，
score 直接落成自增主键。用例里的视频是**绕过发布流程**直接 `videoRepo.CreateWithStats` 建的，
`publish_seq` 为 0，所以只有走「补发号 → 落库 → 用序号写入」这条路径才可能同时满足
「`publish_seq > 0`」与「score 等于它」两个断言；旧写法连第一个都过不了。

**反证**：把 `s.appendApprovedToTimeline(ctx, videoID)` 改回 `_ = s.timelineRepo.Append(ctx, videoID)` 重跑，
用例立刻报 `expected approval to assign a publish_seq, got 0`，随后还原。

### FIX-04 · 消费端 ack 语义

**根因**：三处消费者都是「先 `MarkConsumed` 再干活」。业务一旦失败，`consumed_events` 里
已经有这个 eventID，重投进来第一步就 `isNew=false` 直接返回 nil → ACK，事件永久丢失：
点赞数不加、热度不涨、通知不发。**标记必须是最后一个动作，而不是第一个。**

**做法**：三个消费者各自按副作用的幂等性选路，但共性只有一句 —— **先干活，干成了再标记**。

| 消费者 | 副作用 | 幂等保证 | 顺序 |
|---|---|---|---|
| `feed/fanout.go` | 粉丝 inbox / 作者 outbox 的 `ZAdd` | 天然幂等（同 member 重复写等价） | `IsConsumed` 快判 → 扇出 → `MarkConsumed` |
| `notification/consumer.go` | `notifications` 插入 | `UNIQUE(recipient_id, event_id)` + `ON CONFLICT DO NOTHING` | `IsConsumed` 快判 → 落库 → `MarkConsumed` |
| `feed/hot_score_consumer.go` | 热榜 `ZINCRBY` | **非幂等**，需要事件级守卫 | `ApplyEngagementOnce`（守卫+加分原子化）→ `MarkConsumed` |

**与初版方案的三处偏差（以及为什么）**

1. **通知没有走 `ExecuteWithDeduplication`**。那条路要业务写入与 `consumed_events` 同事务，
   而 `domainnotification.Repository.Create(ctx, n)` 的签名里没有事务句柄 —— 领域接口是刻意不暴露
   `*gorm.DB` 的。硬塞一个 `CreateTx(ctx, tx *gorm.DB, ...)` 会把 gorm 泄进领域层，为了一条
   「插入 + 标记」的原子性付出架构代价。而通知插入本来就已经幂等（唯一键 + `DO NOTHING`），
   重投一次的结果与事务版完全一致，所以选了「先落库、再标记」。
   ⇒ `ExecuteWithDeduplication` 目前仍是**生产代码零调用**的既有死代码，本轮不动它（改动面已经很大），
   留作后续清理。
2. **热榜守卫必须原子化，不能 `SETNX` 完再 `ZINCRBY`**。两步写法有个新的丢事件窗口：`SETNX` 成功、
   紧接着 `ZINCRBY` 失败（连接断开 / OOM），守卫已经落下，重投进来跳过加分 —— 与原来的 bug 一模一样，
   只是丢的层次从「标记」挪到了「守卫」。改成一段 Lua 把 `SET NX EX` 与 `ZINCRBY` 一次执行：
   要么都成，要么都不生效。守卫键 `feed:hot:applied:{event_id}`，TTL 7 天（与 `consumed_events` 的保留期一致）。
3. **必须给重投加预算 + 死信队列**。原来是 `d.Nack(false, true)` 无限重投：修好顺序之后，一条永远
   失败的毒消息会变成无限循环（以前是「一次就丢」，现在是「永远重试」，都不对）。所以给
   `infra/rabbitmq.Consumer` 加了 `AttemptTracker`（默认 5 次预算）与 `<queue>.dead`：
   超预算 → 转投死信队列并 ACK，不再阻塞后面的消息。同时引入 `rabbitmq.Permanent(err)`：
   反序列化失败、`video_id <= 0` 这类「重投一万次也一样」的错误直接进死信，不浪费预算。

**顺带修掉的两个行为**

- 畸形 payload 过去返回 `nil`（悄悄 ACK 掉）。现在返回 `Permanent`，进死信队列可回捞 ——
  静默丢弃数据本身就是这条 bug 的同款，只是不经过 `consumed_events`。
- `consumed_events` 从来没有清理。新增 `PurgeConsumedBefore(ctx, cutoff)` 与
  `migrations/000014_add_consumed_events_retention_index.up.sql`（`idx_consumed_at`，
  避免清理变成全表扫），worker 启动时 + 每小时删 7 天前的行。保留期取自「broker 可能重投多久」，
  不是「业务想留多久」。

**测试**

- 三条「失败不标记」用例，每条都断言三件事：返回 error、`IsConsumed == false`、恢复后重投能补齐副作用。
- `consumer_internal_test.go`：`shouldRequeue` 判定表（第 5 次到预算即死信、`permanent` 永不重试）、
  `Permanent` 穿透 `fmt.Errorf("%w")` 包装、`attemptKey` 在有/无 `MessageId` 时都跨重投稳定、
  计数器过期后从 1 重新计数且被周期清扫。
- `TestHotScoreConsumer_ValidationAndEdgeCases` 里「`video_id=0` 返回 nil」的期望值改为
  `IsPermanent(err) == true`：这是**有意的语义变化**（静默 ACK → 进死信），不是为了把测试改绿。

**反证**（四条，逐条改回去验证用例会红，随后全部还原）

| 改回什么 | 哪条用例红 | 报错 |
|---|---|---|
| fanout 的 `IsConsumed` 换回 `MarkConsumed` | `TestFanoutService_FanoutFailure_DoesNotMarkConsumed` | `event evt-publish-fail-1 was marked consumed even though the fanout failed` |
| notification 的 `IsConsumed` 换回 `MarkConsumed` | `TestConsumer_ProcessingFailure_DoesNotMarkConsumed` | `event ev_persist_fail_1 was marked consumed even though nothing was persisted` |
| 热榜把 `markConsumed` 提到 `ApplyEngagementOnce` 之前 | `TestHotScoreConsumer_ApplyFailure_DoesNotMarkConsumed` | `event ev_apply_fail_1 was marked consumed even though the score was never applied` |
| `shouldRequeue` 的 `<` 放宽成 `<=` | `TestConsumer_ShouldRequeue/attempt_at_the_budget_is_dead-lettered` | `shouldRequeue(attempts=5, permanent=false) = true, want false` |

**验证**

- `go build ./...`、`go vet ./...` 通过；
- `go test -p 1 -count=1 ./internal/...` → 58 个包全 ok；
- `make e2e` → `ok github.com/Tidehyc/gorangefeed/test/e2e 74.458s`。

**遗留取舍**

1. **未做真实 broker 故障演练**：死信队列的落盘行为只有单测层的判定表覆盖，没有「停掉 MySQL → 观察
   worker 重试 5 次 → 消息出现在 `<queue>.dead`」的实测记录。要补这一条得停依赖、盯日志，属于运维演练。
2. **`consumed_events` 与热榜守卫各自 7 天**：两边保留期一致，但它们是两套独立状态。Redis 丢数据时
   热榜守卫消失，此时若同一条消息又被重投（超过 7 天基本不会发生），分数会重复累加一次。
   分数是可再生的近似值，接受这个偏差，不为此引入跨存储事务。
3. **`ExecuteWithDeduplication` 仍是死代码**（见偏差 1）。它把 `*gorm.DB` 写进了 dedup 契约里，
   长期看应该删掉，但删除会波及 2 个 mock 与 1 条针对实现本身的单测，与本轮的 ack 语义修复不是一件事。
