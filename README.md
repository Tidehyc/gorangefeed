# GoRangeFeed

> 短视频 Feed 流后端系统，覆盖「发布 → 分发 → 消费 → 互动 → 通知」全链路闭环，采用整洁架构（Clean Architecture），单体 API + 独立 Worker 双进程。

---

## 一、功能范围

| 模块 | 能力 | 关键实现 |
|---|---|---|
| **账号与鉴权** | 注册、登录、刷新、登出、资料与头像 | 双 Token 轮转 + 刷新令牌重放防护；`role` 字段驱动的 RBAC；封禁后旧 access 立即 403、refresh 直接 401 |
| **视频与上传** | 分片上传、封面、发布、删除、作者作品列表 | 分片位图记录已传块；`complete` 先校验齐全再加分布式锁流式合并并校验 MP4，失败回滚到可重试状态 |
| **Feed 流** | 全局最新、关注流、实时热榜、推荐、点赞榜 | 全局流 L1 公共骨架缓存 + 登录态动态注入；关注流按 5000 粉丝分界推/拉；热榜分钟桶 + `as_of` 快照分页；推荐三路召回 + 同作者滑动打散 + 曝光去重 + offset 游标 |
| **互动** | 点赞/取消、评论（二级回复）、评论点赞、我赞过的 | `RowsAffected` 驱动原子计数；评论 `idempotency_key` 唯一索引防重放 |
| **关注关系** | 关注/取关、粉丝与关注列表、关系探测 | 关系与统计垂直分表；单调 ID 游标翻页；单次查询判定双向关系 |
| **收藏与历史** | 多收藏夹、观看历史、播放进度 | 收藏项与夹分离，按夹分页；历史按 `(user, video)` 唯一并随播放进度更新 |
| **通知** | 点赞/评论/提及/关注通知、未读计数、SSE 推送 | `consumed_events` + `UNIQUE(recipient_id, event_id)` 双层去重；SSE 走一次性 ticket + 跨实例 Redis Pub/Sub |
| **创作者中心** | 草稿、定时发布、发布任务管理 | Redis ZSet 延迟队列 + 分布式锁；发布时分配单调 `publish_seq` |
| **审核中台** | 审核任务、通过/驳回、下架、封禁/解封 | 仅 `admin` 可达；过审后按真实 `publish_seq` 写入时间线；封禁同步失效凭证 |
| **播放 QoS** | 播放配置下发、播放质量事件上报 | 按网络类型自适应预加载条数与缓冲阈值；首帧耗时、卡顿次数与时长导出为 Prometheus 指标 |

---

## 二、架构简述

GoRangeFeed 遵循**整洁架构（Clean Architecture）**分层设计，领域层（Domain）保持纯粹零外部框架依赖，通过依赖倒置驱动基础设施；端到端（E2E）黑盒测试与模块内白盒单测严格分流：

* **API 入口 (`cmd/api`)**：依赖注入装配、探针中间件、统一信封响应、路由组分发与平滑优雅停机；
* **Worker 入口 (`cmd/worker`)**：独立进程，负责 Outbox 事件派发、异步消费、延迟任务调度与周期性维护（时间线裁剪、`consumed_events` 保留期清理）；
* **领域核心 (`internal/domain/`)**：纯业务实体、值对象与 Repository/TokenStore 抽象契约，共 12 个领域（`account`、`video`、`upload`、`creator`、`feed`、`interaction`、`relation`、`asset`、`notification`、`outbox`、`moderation`、`playback`）；
* **应用服务 (`internal/service/`)**：业务用例编排、状态机控制与事务协调（如双 Token 轮转、审核过审流水线、事件消费）；
* **传输接口 (`internal/http/`)**：Gin 控制器、请求/响应 DTO、参数绑定校验与路由组挂载；
* **持久化基础设施 (`internal/infra/persistence/`)**：GORM MySQL 实体仓储与 Redis Lua 脚本会话存储实现；
* **基础设施 (`internal/infra/`)**：MySQL、Redis、RabbitMQ 连接池装配、JWT 管理、本地头像与媒体存储、配置加载与日志监控；
* **测试中心 (`test/`)**：
  * `test/e2e/`：独立运行的端到端黑盒生命周期测试；
  * `test/benchmark/`：Feed 与互动链路压测基准；
  * `test/testutil/`：集成测试脚手架与 HTTP 辅助库；
  * 模块单元测试（`*_test.go`）保留在各层代码旁，便于白盒测试。

### 异步链路

```
业务写路径 ──事务──> outbox（本地消息表）
                        │  Worker 租约抢占 + 指数退避
                        ▼
             exchange.gorangefeed.events（Topic）
        ┌───────────────┼────────────────┐
        ▼               ▼                ▼
  following_feed     hot_rank      notifications
   （写扩散扇出）   （热度打分）    （通知落库 + SSE）
        └───────────────┴────────────────┘
                        │ 失败重投（上限 5 次）
                        ▼
                  <queue>.dead（死信）
```

三个消费者统一遵守一条纪律：**先执行幂等副作用，成功后才写 `consumed_events` 标记**。顺序反过来，一次业务失败就会变成「重投撞上标记 → 直接 ACK → 事件永久丢失」。

---

## 三、技术栈选型

| 基础设施 / 组件 | 选型 | 宿主机端口映射 |
|---|---|---|
| 语言框架 | Go 1.26+ / Gin | `:8080` (API) / `:9091` (Worker Metrics) |
| 关系型数据库 | MySQL 8.0 / GORM | `:3307` (映射容器 3306) |
| 缓存与索引 | Redis 7.0 | `:6380` (映射容器 6379) |
| 消息队列 | RabbitMQ 3-management | `:5673` (AMQP) / `:15673` (管理后台) |
| 监控中心 | Prometheus 3.1.0 | `:9090` (Prometheus Web UI) |
| 数据库迁移 | golang-migrate，14 个版本（`000001`~`000014`），脚本在 `migrations/` | — |
| 日志与监控 | `log/slog` + Prometheus | API `:8080/metrics` / Worker `:9091/metrics` |

---

## 四、10 分钟本地快速上手

### 1. 环境准备
确保本地已安装：
* [Go 1.26+](https://golang.org/dl/)
* [Docker & Docker Compose](https://www.docker.com/)
* `make` 工具

### 2. 启动基础依赖容器
```bash
# 一键启动 MySQL 8.0, Redis 7.0, RabbitMQ 3 与 Prometheus 监控
make up
```

### 3. 执行数据库初始迁移
```bash
# 执行数据库迁移（初始化 schema_migrations）
make migrate
```

### 4. 启动服务
```bash
# 运行 API 服务 (监听 :8080)
make run

# 或在另一终端运行 Worker 服务 (监听 Metrics :9091)
make worker
```

> 需要管理员权限的接口（审核、下架、封禁）要求 `users.role = 'admin'`，首个管理员用 `make promote-admin ID=<user_id>` 提权。

### 5. 探针与指标验证
```bash
# 检查存活探针 (Liveness)
curl http://127.0.0.1:8080/healthz
# 响应: {"status":"ok"}

# 检查就绪探针 (Readiness，依赖 MySQL/Redis/RabbitMQ)
curl http://127.0.0.1:8080/readyz
# 响应: {"components":{"mysql":"ok","rabbitmq":"ok","redis":"ok"},"status":"ready"}

# 查看 API 导出的 Prometheus 指标
curl http://127.0.0.1:8080/metrics

# 查看 Worker 导出的 Prometheus 指标
curl http://127.0.0.1:9091/metrics

# 访问 Prometheus Web 控制台查看拉取状态与监控大盘
# 浏览器打开: http://127.0.0.1:9090
```

### 6. 测试与代码校验
```bash
# 静态代码检查
make vet

# 运行全量单元与集成测试（串行执行，带覆盖率）
make test

# 并发安全检测
make test-race

# 端到端生命周期测试（需要 make up 起的依赖，约 80s）
make e2e

# 编译所有二进制产物至 bin/ 目录
make build
```

---

## 五、核心接口一览

全部接口挂在 `/api/v1` 下，下表路径省略该前缀。需要登录的接口使用 `Authorization: Bearer <access_token>`；
Feed 读接口大多为软鉴权（游客可访问，带 token 时注入 `is_liked` / `is_followed`），`/feed/following` 与 `/users/me/*` 必须登录。

| 模块 | 代表接口 | 鉴权 |
|---|---|---|
| 账号 | `POST /auth/register`、`POST /auth/refresh`、`GET /users/me`、`PUT /users/me/password`、`DELETE /users/me` | 公开 / 登录 |
| 视频 | `GET /videos/:videoId`、`DELETE /videos/:videoId`、`GET /users/:userId/videos` | 软鉴权 / 登录 |
| 分片上传 | `POST /videos/chunk/init`、`POST /videos/chunk/upload`、`POST /videos/chunk/complete` | 登录 |
| Feed | `GET /feed/global`、`GET /feed/following`、`GET /feed/hot`、`GET /feed/recommend`、`GET /feed/like-rank` | 软鉴权（关注流需登录） |
| 话题 | `GET /tags/:tag/videos` | 软鉴权 |
| 互动 | `POST /videos/:videoId/like`、`POST /videos/:videoId/comments`、`GET /users/me/likes` | 登录 |
| 关注 | `POST /users/:userId/follow`、`GET /users/:userId/followers` | 登录 / 软鉴权 |
| 收藏与历史 | `POST /user/favorites`、`GET /user/history`、`POST /videos/:videoId/history` | 登录 |
| 通知 | `GET /notifications/unread-count`、`POST /notifications/sse/ticket`、`GET /notifications/stream` | 登录 / ticket |
| 创作者 | `POST /creator/drafts`、`POST /creator/publish-scheduled` | 登录 |
| 管理 | `GET /admin/reviews`、`POST /admin/videos/:id/review`、`POST /admin/users/:id/ban` | 登录 + `role=admin` |
| 播放 | `GET /playback/config`、`POST /playback/events` | 软鉴权 |

---

## 六、核心工程约束

1. **响应格式统一**：
   * 成功：`{"data": ..., "error": null}`
   * 失败：`{"data": null, "error": {"code": "...", "message": "..."}}`
2. **禁止 `clientFoundRows`**：
   * 点赞/互动链路依赖 MySQL `RowsAffected` 进行计数判定，配置加载时强校验严禁开启 `clientFoundRows=true`。
3. **消费端 ack 语义**：
   * 先执行业务副作用，成功后才写 `consumed_events`；标记是收尾动作，不是开场动作；
   * 副作用必须幂等：`ZADD` 天然幂等，通知靠 `UNIQUE(recipient_id, event_id)` + `ON CONFLICT DO NOTHING`，
     `ZINCRBY` 靠事件级守卫 `feed:hot:applied:{event_id}` 与加分打包在同一条 Lua 脚本里；
   * 同一消息最多重投 5 次，超限或遇到 `Permanent`（畸形 payload）转投 `<queue>.dead`，禁止无限回队。
4. **分页游标**：
   * 一律透明 Base64 编码，客户端不得解析；全局流/关注流/已赞列表用单调 ID 游标，推荐用 `{offset, seed}`（会话内已消费条数），热榜用 `{as_of, offset}`（对齐整分钟快照）；
   * 同一游标重复请求结果必须逐项相同，翻页不得重复或漏项。
5. **写路径限流**：
   * 按 UID 计：点赞/评论/关注各 30 次/分钟，一组一个桶；限流器挂在鉴权之后（否则退化成按 IP）。
6. **迁移脚本不可变性**：
   * `migrations/` 历史已合并脚本严禁更改或删除，仅支持新增版本递增脚本；`bash scripts/check_migrations.sh` 校验。

---

## 七、文档索引

| 文档 | 内容 |
|---|---|
| [`docs/REQUIREMENTS.md`](docs/REQUIREMENTS.md) | 功能与非功能需求、验收口径 |
| [`docs/DESIGN.md`](docs/DESIGN.md) | 详细设计：表结构、缓存键、异步链路 |
| [`docs/ARCHITECTURE_DECISIONS.md`](docs/ARCHITECTURE_DECISIONS.md) | 分模块架构决策与同类项目对比 |
| [`docs/PROGRESS.md`](docs/PROGRESS.md) | 里程碑进度（M0~M10 完成，M11~M13 规划中） |
| [`docs/BENCHMARK_REPORT.md`](docs/BENCHMARK_REPORT.md) | 压测数据与瓶颈分析 |
| [`docs/FIX_TODO.md`](docs/FIX_TODO.md) | 缺陷清单、修复实录与遗留取舍 |
| [`scripts/README.md`](scripts/README.md) | 运维脚本说明 |

---

## 八、目录结构

```
cmd/            api / worker / migrate 三个入口
internal/
  domain/       12 个领域的实体与契约（零框架依赖）
  service/      用例编排、状态机、事务与消费者
  http/         Gin 控制器、DTO、路由组
  infra/        持久化、Redis、RabbitMQ、调度器、中间件、配置
migrations/     14 个版本的正向/回滚脚本
test/           e2e 黑盒生命周期、benchmark 压测、testutil 脚手架
configs/        本地配置
scripts/        迁移校验等运维脚本
```
