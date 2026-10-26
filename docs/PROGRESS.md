# GoRangeFeed 项目进度跟踪

> 本文档用于跟踪 GoRangeFeed 项目的阶段目标与任务推进状态。

---

## 一、状态与规则说明

| 状态 | 说明 |
|---|---|
| `TODO` | 待开始 |
| `DOING` | 开发中（代码落盘但尚未通过测试或验证） |
| `DONE` | 验收通过（具备可通过的单元测试/集成测试或自动化验证） |
| `BLOCKED` | 遇到阻塞，需在验收栏说明阻塞原因 |
| `DROPPED` | 废弃或取消，需在验收栏说明原因 |

---

## 二、当前项目快照

| 监控项 | 当前状态 |
|---|---|
| **当前阶段** | **M2 视频单文件上传与流式发布已全量完成（全链路 E2E 验收通过）** |
| **完成进度** | M0 阶段：15 / 15 项（100%） · M1 阶段：14 / 14 项（100%） · M2 阶段：9 / 9 项（100%） |
| **进行中任务** | M2 所有 9 项任务（含数据表迁移、领域模型、仓储持久化、Service业务用例、MP4嗅探上传、HTTP 206 Range 流式点播、HTTP 控制器与全生命周期 E2E 验证）已全部验收通过；准备进入下一阶段 |
| **阻塞事项** | 无 |
| **下一步行动** | 评估推进 M2.5（分片上传与断点续传）或直接开启 M3（全局最新 Feed 流与 Redis 冷热拼接架构） |

---

## 三、里程碑总览

| 里程碑 | 主题 | 关联需求 | 状态 | 核心完成标准 |
|---|---|---|---|---|
| **M0** | 工程骨架与基础设施装配 | NFR-MAINT-*, NFR-OBS-01 | `DONE` | `go vet` + `go test` 绿灯；`docker compose` 一键启动；`/readyz` 正确反映依赖状态，`/metrics` 输出标准指标 |
| **M1** | 账号、双 Token 与安全中间件 | FR-ACC-01~06, NFR-SEC-07 | `DONE` | 登录、刷新、登出及旧 Token 撤销全流程通过；软鉴权与频控限流测试通过 |
| **M2** | 视频单文件上传与流式发布 | FR-VID-01~05, NFR-EXT-01 | `DONE` | E2E 单测跑通上传、发布、206 Range 分段点播、详情、游标分页与下架；`videos` 与 `video_stats` 事务一致 |
| **M2.5**| 分片上传与断点续传（二期） | FR-VID-06~07 | `TODO` | 200MB 文件断网 3 次重试续传成功；MySQL 位图权威状态保证无并发覆盖 |
| **M3** | 全局最新 Feed 流 | FR-FEED-01, 03, 04 | `TODO` | 游标翻页无重无漏；Redis 异常时自动平滑降级 DB；下架内容补页正确 |
| **M4** | 互动、推拉关注流与 Outbox 派生 | FR-INT-*, FR-REL-*, FR-FEED-02 | `TODO` | 高并发点赞计数不丢；评论支持 `idempotency_key` 幂等防重放；关注流推拉合并正确；Outbox 租约与死信运转正常 |
| **M5** | 系统通知与单实例 SSE | FR-NOT-01~05 | `TODO` | 点赞后通知 P99 < 1s 异步落库；SSE 在线推送与断线补拉验证完整 |
| **M5.5**| 跨实例 SSE 广播（二期） | FR-NOT-06 | `TODO` | 多实例下 Pub/Sub 广播到达；断连期间通知通过补拉恢复 |
| **M6** | 性能压测与候选池精排（二期）| NFR-PERF-* | `TODO` | 核心链路达到性能设计指标或产出基准分析报告 |

---

## 四、M0 骨架任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M0-01** | 初始化目录结构与 Go Module | — | `go.mod` 模块名为 `github.com/Tidehyc/gorangefeed`；目录结构与设计方案一致（包含 `web/README.md` 占位） | `DONE` |
| **M0-02** | 配置加载与启动校验 | M0-01 | 覆盖单元测试：必填项校验、环境覆盖、`.env` 自动装载、非法值拦截、禁止 `clientFoundRows` | `DONE` |
| **M0-03** | 结构化日志组件（`log/slog`） | M0-01 | 支持 Text/JSON 格式切换；日志级别可配；零额外三方依赖 | `DONE` |
| **M0-04** | 统一 API 响应信封与错误码 | M0-01 | 成功/失败 Envelope 格式一致；未识别错误兜底为 `internal` | `DONE` |
| **M0-05** | 接入 `golang-migrate` 数据库迁移 | M0-06 | 本地数据库执行初始迁移；验证版本推进与 dirty 状态管理机制 | `DONE` |
| **M0-06** | 基础设施连接装配（MySQL/Redis/MQ） | M0-02 | 依赖不可用时快速失败并给出可读原因；连接池配置生效 | `DONE` |
| **M0-07** | HTTP 服务骨架与探针中间件 | M0-06 | 挂载 `RequestID` 与 `AccessLog` 中间件；暴露 `/healthz`、`/readyz` 与 Prometheus `/metrics` 端点；停用 Redis 时 readyz=503 而 healthz=200 | `DONE` |
| **M0-08** | API 服务入口与优雅停机 | M0-07 | 监听 SIGTERM 信号，在途请求处理完毕后按序释放资源，无 Panic | `DONE` |
| **M0-09** | Worker 异步服务入口 | M0-06 | 独立进程入口，支持 Prometheus `:9091/metrics` 导出，能正常启动与优雅退出 | `DONE` |
| **M0-10** | 配置文件模板与环境变量示例 | M0-02 | `configs/config.yaml`、`configs/prometheus.yml` 与 `.env.example` 完整齐备 | `DONE` |
| **M0-11** | 本地 Compose 编排文件 | — | 宿主机端口映射：MySQL 3307、Redis 6380、RabbitMQ 5673、Prometheus 9090 | `DONE` |
| **M0-12** | Makefile 工程脚本 | M0-11 | 支持 `make up / down / migrate / run / test / vet` | `DONE` |
| **M0-13** | GitHub Actions CI 工作流 | M0-12 | PR/Push 自动执行 `vet` + `test` + `build`；校验历史迁移脚本不变性 | `DONE` |
| **M0-14** | 项目快速上手 README | — | 包含 10 分钟本地构建启动指引与架构简述 | `DONE` |
| **M0-15** | M0 端到端全流程验证 | M0-08~13 | 全新环境下 `make up` → `make migrate` → `make run` → 探针与 metrics 正常 → `make test` 全绿 | `DONE` |

> **边界约定**：M0 阶段不建业务数据表、不写业务 API Handler，数据库中仅初始化 `schema_migrations` 表。

---

## 五、M1 账号与鉴权任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M1-01** | 用户数据表迁移与版本管理 | M0-05 | 编写 `000002_create_users_table`；包含用户名唯一索引、状态枚举与时间戳；通过 migrate up/down 与历史不变性校验 | `DONE` |
| **M1-02** | JWT TokenManager 基础设施实现 | M0-02 | 引入 `golang-jwt/jwt/v5`；支持 Access Token（短效）与 Refresh Token（长效）签发与验签；覆盖过期/篡改单测 | `DONE` |
| **M1-03** | 密码强度校验与 bcrypt 哈希 | — | 覆盖 8~32 位、字符数字混合校验；用户名正则合规校验；bcrypt 加密与对比验证 | `DONE` |
| **M1-04** | 账号仓储与 Redis Token 会话管理 | M1-01, M1-02 | GORM 增查与资料更新；基于 Redis Lua 脚本原子实现 Refresh Token 轮转与重放探测（重放即刻销毁会话）；Access 黑名单 | `DONE` |
| **M1-05** | 账号领域 Service 业务闭环 | M1-02~04 | 注册、登录、Token 轮转、登出与资料维护逻辑；覆盖完整分支与重放攻击防御单测 | `DONE` |
| **M1-06** | JWT 鉴权与软鉴权中间件 | M1-02, M1-04 | 实现 `JWTAuth` 与 `SoftJWTAuth`；支持 Header 与 Query `?token=` 提取；拦截黑名单 Token；软鉴权兼容游客与 EventSource | `DONE` |
| **M1-07** | Redis 滑动窗口限流中间件 | M0-06 | 基于 Redis ZSet 实现原子真·滑动窗口限流；支持按 IP 限制登录注册（10次/分）、按 UID 限制高频操作（30次/分）；超限返回 `rate_limited` | `DONE` |
| **M1-08** | 本地头像存储与媒体静态路由 | M0-07 | 限制 ≤10MB 与 `.jpg/.jpeg/.png/.webp` 格式；UUID 命名防路径穿越；通过 `/media/avatars/*` 路由提供安全访问 | `DONE` |
| **M1-09** | 账号域 API Handler 与输入校验 | M1-05, M1-08 | 暴露注册、登录、刷新、登出、当前用户资料（`/me`）、公开资料（`/:userId` 不暴露隐私）与头像上传接口；统一 Envelope 与错误码 | `DONE` |
| **M1-10** | 路由挂载与依赖装配 | M1-06~09 | 在 `cmd/api` 完成依赖注入，挂载 `/api/v1/auth/*` 与 `/api/v1/users/*` 路由组，绑定限流与鉴权中间件 | `DONE` |
| **M1-11** | M1 端到端全流程集成测试 | M1-10 | 覆盖注册 → 登录 → 换 Token → 重放探测拦截 → 资料更新 → 公开主页访问 → 媒体路由获取头像 → 登出 → 黑名单拦截 → 频控限流全生命周期 E2E 验证 | `DONE` |
| **M1-12** | 整洁分层架构重构（方案 1） | M1-11 | 消除垂直大包，清晰划分 `domain`（零外部依赖）、`service`、`http`、`infra/persistence`；消除所有循环依赖 | `DONE` |
| **M1-13** | 迁移集成测试表隔离 | M1-12 | 支持迁移表参数化，单元测试采用 `schema_migrations_test` 并在完成后自动 drop，避免多 package 并发测试污染 | `DONE` |
| **M1-14** | 测试目录规范化分流（方案 A） | M1-13 | 遵循 Go 标准将 E2E 黑盒测试抽离至根目录 `test/e2e/account_lifecycle_test.go`，白盒单测保留在各包旁；搭建全模块架构骨架与 `doc.go` | `DONE` |

---

## 六、M2 视频核心与流式发布任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M2-01** | 视频主表与统计表迁移脚本 | M0-05 | 编写 `000003_create_videos_table`，垂直拆分 `videos` 与 `video_stats`（隔离高频行锁）；引入 `idempotency_key` 并建立唯一索引 `uk_author_idempotency(author_id, idempotency_key)` 支持发布幂等；建立 `(status, id DESC)`、`(author_id, status, id DESC)` 与 `published_at` 索引；通过 migrate 历史一致性与回滚校验 | `DONE` |
| **M2-02** | 视频领域模型与仓储契约定义 | — | 在 `internal/domain/video` 定义 `Video`（含状态机：草稿/已发布/已下架）、`VideoStats`、`VideoDetail` 实体；声明 `VideoRepository`（含 `SaveWithStats`、`FindByAuthorAndIdempotencyKey`、`ListByAuthorCursor` 等）接口，定义领域错误常量（如 `ErrVideoNotFound`, `ErrVideoForbidden`, `ErrDuplicateIdempotencyKey`），保持零外部依赖 | `DONE` |
| **M2-03** | 视频存储管理与 HTTP 206 Range 流媒体服务 | M0-07 | 扩展 `internal/infra/storage` 支持视频本地存储与路径防穿越（UUID 隔离）；在路由层挂载 `/media/videos/*` 静态路由，基于 `http.ServeContent` 原生支持 HTTP 206 Partial Content（支持 Range 请求头、Content-Range、分段传输与 416 越界校验） | `DONE` |
| **M2-04** | 视频单文件上传与 MIME/MagicNumber 严格校验 | M2-03 | 限制 MP4 格式、大小 ≤200MB（`http.MaxBytesReader` 请求体截断防打满内存）；执行二进制 Magic Number 嗅探（ftyp 标识）与 MIME 双重拦截；**支持异常失败自动清理机制（校验/解析失败即刻删除磁盘残留，杜绝脏文件）** | `DONE` |
| **M2-05** | 视频仓储与统计持久化实现 | M2-01, M2-02 | 在 `internal/infra/persistence/video` 基于 GORM 实现仓储；单事务原子创建 `videos` 并初始化 `video_stats`（计数全为 0）；实现基于唯一键冲突拦截的发布幂等查询；实现作者作品流单调 ID 游标查询（`id < cursor` 倒序）与状态变更 | `DONE` |
| **M2-06** | 视频领域 Service 核心业务用例 | M2-02, M2-04, M2-05 | 在 `internal/service/video` 实现视频上传、幂等发布（命中幂等键安全返回原视频）、详情组装（回填统计数据与作者公开资料，下架拦截）、**作者作品流高性能单调 ID 游标分页（杜绝 offset 深分页与并发脏读）**、下架（仅作者可下架，保持操作幂等）；覆盖单元测试与 Mock 测试 | `DONE` |
| **M2-07** | 视频 HTTP 控制器与输入校验 | M2-06 | 在 `internal/http/video` 暴露上传、发布（支持 `Idempotency-Key` 请求头/参数）、详情（软鉴权兼容游客与登录态）、作者流（游标入参与 `next_cursor` 返回）、下架接口；统一 API Envelope 与规范化错误码；编写 Handler 单元测试 | `DONE` |
| **M2-08** | 路由挂载与依赖注入装配 | M2-07 | 在 `cmd/api/main.go` 中完成 Repository/Service/Handler 依赖组装；注册 `/api/v1/videos/*` 与 `/api/v1/users/:userId/videos` 路由组，绑定 JWT 鉴权与软鉴权中间件；挂载 `/media/videos/*` 流媒体路由 | `DONE` |
| **M2-09** | M2 视频全生命周期 E2E 集成测试 | M2-08 | 在 `test/e2e/video_lifecycle_test.go` 中验证上传 → 格式嗅探拦截与脏文件清理 → 事务发布与幂等重放 → HTTP 206 Range 分段流式播放（验证 bytes 0-1023 分段与 Content-Range） → 详情与作者流游标分页 → 越权下架拦截 → 作者本人下架 → 下架后详情 404 的全生命周期自动化验证 | `DONE` |

---

## 七、后续里程碑任务概览

- **M2.5 分片续传**：上传会话表与状态机 · 分片 Init/Upload/Status/Complete · 位图控制（MySQL 权威 + Redis 读加速）· 文件合并并发锁 · 针对性续传测试。
- **M3 全局 Feed**：DB 基线查询与单调 ID 游标 · Redis ZSet 时间线写入与原子裁剪（Lua）· Floor 计算与冷热拼接 · 下架过滤与游标推进 · 逻辑过期 + Singleflight 页缓存 · 降级策略 · 8 组边界测试。
- **M4 互动与 Outbox**：点赞/关注表与关系统计 · 点赞事务（`RowsAffected` 驱动）· 评论（基于客户端 `idempotency_key` 唯一键防重放）与关注 · Outbox 表与事务落库 · 租约派发调度器 · 消费端幂等（`consumed_events`）· 关注流推拉结合分发与多路归并 · 对应测试。
- **M5 通知与实时推送**：系统通知表 · Outbox 消费生成通知 · 未读计数与列表 · 标记已读 · SSE Hub 与短期 Ticket 鉴权 · 60s 重叠窗口断线补拉 · 对应测试。
