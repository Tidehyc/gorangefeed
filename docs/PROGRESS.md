# GoRangeFeed 项目进度跟踪

> 本文档用于跟踪 GoRangeFeed 项目的阶段目标与任务推进状态。  
> 详细系统架构设计方案请参阅 [docs/DESIGN.md](file:///home/tide/GoProjects/GOrangeFeed/docs/DESIGN.md)；  
> 跨项目对比分析与技术决策记录请参阅 [docs/ARCHITECTURE_DECISIONS.md](file:///home/tide/GoProjects/GOrangeFeed/docs/ARCHITECTURE_DECISIONS.md)。

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
| **当前阶段** | **二期架构演进进行中：M7 创作中心与高可靠发布时序已全量通过验收（准备推进 M8: 视频收藏夹与播放历史资产中心）** |
| **完成进度** | **一期核心闭环**：M0~M6 阶段 89 / 89 项（100%） · **二期进行中**：M7 阶段 8 / 8 项（100.0%） · M8~M10 规划中 |
| **进行中任务** | M7 全生命周期通过验收，准备推进 M8 视频收藏夹与播放历史资产中心 (M8-01~M8-06) |
| **阻塞事项** | 无 |
| **下一步行动** | 推进 M8 阶段：编写 000011_create_favorites_and_history_tables 迁移脚本与资产中心领域模型 |



---

## 三、里程碑总览

| 里程碑 | 主题 | 关联需求 | 状态 | 核心完成标准 |
|---|---|---|---|---|
| **M0** | 工程骨架与基础设施装配 | NFR-MAINT-*, NFR-OBS-01 | `DONE` | `go vet` + `go test` 绿灯；`docker compose` 一键启动；`/readyz` 正确反映依赖状态，`/metrics` 输出标准指标 |
| **M1** | 账号、双 Token 与安全中间件 | FR-ACC-01~06, NFR-SEC-07 | `DONE` | 登录、刷新、登出及旧 Token 撤销全流程通过；软鉴权与频控限流测试通过 |
| **M2** | 视频单文件上传与流式发布 | FR-VID-01~05, NFR-EXT-01 | `DONE` | E2E 单测跑通上传、发布、206 Range 分段点播、详情、游标分页与下架；`videos` 与 `video_stats` 事务一致 |
| **M2.5**| 分片上传与断点续传（二期） | FR-VID-06~07 | `DONE` | 200MB 文件断网重试续传成功；基于 Redis Bitmap 与分布式锁杜绝并发脏写与重复合并；MySQL 权威状态机保证一致性 |
| **M3** | 全局最新流与播放曝光上报 | FR-FEED-01, 03, 04, FR-VID-08 | `DONE` | 游标翻页无重无漏；Redis 异常时自动平滑降级 DB；下架内容补页正确；上报播放行为驱动 `view_count` 自增与曝光打底 |
| **M4** | 互动、推拉关注流与已赞作品 | FR-INT-*, FR-REL-*, FR-FEED-02, 07 | `DONE` | 点赞/评论/关注事务一致性与计数防丢；客户端唯一键评论幂等防重放与热评排序；互粉好友判定；Outbox 本地消息表分布式租约调度与 RabbitMQ 可靠投递；消费端事务去重；关注流推拉结合（普通写扩散+大V读扩散多路归并）与冷启动回源；全局流 L1 骨架共享与个性化互动态动态装配；历史累计高赞榜复合游标分页 |
| **M4.5**| 实时热榜与轻量推荐流（进阶流） | FR-FEED-05, FR-FEED-06 | `DONE` | 60分钟滑动窗口互动热度衰减（ZSet分钟桶）；as_of+offset稳定快照分页；热门/最新/探索多路召回、曝光去重与同作者打散；全生命周期 E2E 测试绿灯 |
| **M5** | 系统通知与分布式跨实例 SSE | FR-NOT-01~07 | `DONE` | 点赞/评论/关注/提及通知 P99 < 1s 异步落库；基于 Redis Pub/Sub 的跨实例广播秒级到达；SSE 在线推送与 60s 重叠窗口基于 Last-Event-ID 断线补拉验证完整 |
| **M6** | 性能压测与播放 QoS 体验监控 | NFR-PERF-*, FR-QOS-* | `DONE` | 核心链路达到性能设计指标并产出基准分析报告；端侧预加载策略建议与播放卡顿/首帧 QoS 上报监控 |
| **M7** | 创作中心与高可靠发布时序 | FR-CRT-*, NFR-REL-03 | `DONE` | 草稿箱全生命周期管理；秒级准点定时发布延迟队列调度；分布式发号器生成 `publish_seq` 彻底消除时间线翻页空洞 |
| **M8** | 视频收藏夹与播放历史资产中心 | FR-INT-08, FR-USR-04 | `TODO` | 默认与自定义多收藏夹；播放历史滑动窗口保留与断点续播；高权重收藏事件正向加权热榜与推荐召回 |
| **M9** | 内容合规与安全审核中台 | NFR-SEC-08, FR-ADM-* | `TODO` | 先审后发/先发后审双轨制；Aho-Corasick 高效敏感词机审引擎；运营后台违规下架与用户封禁处罚 |
| **M10**| 全栈 Web 短视频流式演示客户端 | FR-WEB-* | `TODO` | 响应式 Web 视频流上下滑播放；双击点赞动画；实时 SSE 通知浮窗展示；QoS 观测面板展示 |

---


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

## 七、M2.5 视频分片上传与断点续传任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M2.5-01** | 分片上传会话表迁移与版本管理 | M0-05 | 编写 `000004_create_upload_sessions_table`，定义 `upload_sessions` 权威持久化表（包含 `upload_id` 唯一索引、`(user_id, file_hash)` 断点定位索引、`status` 状态枚举、`total_chunks`、`chunk_size`、`expire_at` 等）；通过 migrate up/down 回滚验证 | `DONE` |
| **M2.5-02** | 分片上传领域模型与仓储契约定义 | — | 在 `internal/domain/upload` 定义 `UploadSession` 聚合根（包含 `Uploading/Merging/Completed/Expired` 状态机流转与位图计算）、`UploadSessionRepository` 接口及领域错误常量，保持零外部依赖 | `DONE` |
| **M2.5-03** | 基于 Redis Bitmap 与分布式锁的高并发状态控制 | M0-06 | 在 `internal/infra/persistence/upload` 实现仓储；引入 Redis 原生 `SETBIT` / `BITCOUNT` 彻底解决并发上传切片时的脏写覆盖问题；实现基于 Redis `NX EX` 的分片合并互斥锁，配合 MySQL 状态机 CAS 防重入 | `DONE` |
| **M2.5-04** | 分片存储管理与原子流式拼接引擎 | M2-03 | 扩展 `internal/infra/storage`，实现切片安全落盘（`uploads/tmp/{upload_id}/{index}` 路径防穿越）、分片 MD5 校验、低内存流式追加拼接；合并完成后严格执行 MP4 Magic Number 二进制嗅探校验，合规即移动至最终路径并安全清理临时分片目录，校验失败即刻回滚清理 | `DONE` |
| **M2.5-05** | 分片续传领域 Service 核心业务用例 | M2.5-02~04 | 在 `internal/service/upload` 实现 `InitSession`（存在未过期相同文件哈希会话时直接断点恢复）、`UploadChunk`（幂等覆盖已上传切片）、`GetStatus`（返回已完成切片索引清单）、`Complete`（并发防重入校验、流式合并与状态终态流转）；覆盖单测与并发 Mock 测试 | `DONE` |
| **M2.5-06** | 分片续传 HTTP 控制器与路由装配 | M2.5-05 | 在 `internal/http/upload` 暴露 `/api/v1/videos/chunk/init`、`upload`、`status`、`complete` 接口；绑定 JWT 认证；统一 API 响应信封；在 `cmd/api/main.go` 完成依赖注入与路由挂载；编写 Handler 单测 | `DONE` |
| **M2.5-07** | M2.5 分片断点续传全生命周期 E2E 集成测试 | M2.5-06 | 在 `test/e2e/chunk_upload_test.go` 中验证会话初始化 → 模拟断网（仅传部分切片） → 再次 Init/Status 断点恢复 → 多切片并发上传（验证位图并发安全性） → 重复分片上传幂等性 → 并发 Complete 合并防重入拦截 → 合成视频 HTTP 206 流式点播验证 → 伪造非法切片合并失败自动清理全链路 | `DONE` |

---

## 八、M3 全局最新 Feed 流任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M3-01** | Feed 领域模型、游标分页对象与仓储契约定义 | — | 在 `internal/domain/feed` 定义 `FeedItem`、`FeedPage`、`FeedCursor`、`CachedPage`（逻辑过期模型）、`ViewEvent`（播放与曝光实体）、`TimelineRepository`、`PageCacheRepository`、`CardCacheRepository`、`FeedDBRepository`、`ExposureRepository` 接口及领域错误常量，保持零外部依赖与单元测试覆盖 | `DONE` |
| **M3-02** | Redis ZSet 全局时间线与 Lua 原子裁剪落地 | M0-06, M3-01 | 在 `internal/infra/persistence/feed` 实现 `TimelineStore`；封装 `timeline:global`（score=video.id）与独立 `timeline:floor`；基于 Lua 脚本原子执行 `ZREMRANGEBYSCORE` 裁剪与 Floor 更新；支持 `ZREVRANGEBYSCORE (cursor -inf LIMIT 0 N` 倒序游标查询与冷启动重建 | `DONE` |
| **M3-03** | 多级缓存体系（L1 逻辑过期+Singleflight、L3 卡片、L4 实时计数）实现 | M0-06, M3-01 | 采纳 GCFeed 模式实现 L1 页缓存（`feed:page:{scene}:{viewer}:{cursor}:{limit}:{rankVersion}`，含逻辑过期结构体与 `singleflight.Group` 进程内防击穿并发合并，30s 容忍平滑返回）；实现 L3 视频卡片（`feed:card:{videoID}`，15m TTL）与 L4 统计缓存（`feed:stat:{videoID}`，15s TTL）的批量 MGET/MSET 存取 | `DONE` |
| **M3-04** | MySQL 冷数据兜底查询与 Floor 动态分界持久化 | M2-01, M3-01 | 在 `internal/infra/persistence/feed` 基于 GORM 实现 `FeedDBRepository`；实现基于 `idx_status_id (status, id DESC)` 的单调 ID 游标翻页查询（`id < cursor`）；实现 72h Floor 动态分界计算 `SELECT MIN(id) FROM videos WHERE status = 1 AND published_at >= NOW() - INTERVAL 72 HOUR` | `DONE` |
| **M3-05** | Feed 业务编排、冷热拼接与播放曝光上报闭环 | M3-01~04 | 在 `internal/service/feed` 编排读链路：L1 命中返回 → L2 时间线拉取 candidate IDs → 下架状态过滤与 `next_cursor` 推进至最后扫描项（单次扫描上限 500 条） → **冷热无缝拼接（热区不足时计算 remainLimit 从 MySQL 补齐）** → 越过 Floor 或 Redis 异常自动平滑降级 MySQL 冷区 → L3/L4 缺失异步批量回填 → 回写 L1 页缓存；**实现播放与曝光行为处理（驱动 `video_stats.view_count` 原子累加，异步写入 Redis 近期用户曝光集合为推荐去重打底）**；提供视频发布后的时间线追加钩子；覆盖单测与 Mock 测试 | `DONE` |
| **M3-06** | Feed HTTP 控制器与软鉴权路由挂载 | M3-05 | 在 `internal/http/feed` 暴露 `GET /api/v1/feed/global` 全局流接口与 `POST /api/v1/videos/:videoId/view` 播放曝光上报接口；挂载 `SoftJWTAuth` 中间件（匿名返回通用流，登录态解析 `userID` 注入上下文）；支持 `cursor` 与 `limit`（默认 10，最大 30）参数校验；统一 API Envelope；编写 Handler 单元测试 | `DONE` |
| **M3-07** | 路由挂载与依赖装配联动 | M3-06 | 在 `cmd/api/main.go` 完成 Feed 仓储、多级缓存、Service 与 Handler 的依赖组装；注册 `/api/v1/feed/*` 与 `/api/v1/videos/:videoId/view` 路由；在视频发布成功后联动触发时间线同步更新（写失效记录 Warn 日志不阻断发布）；探针检查生效 | `DONE` |
| **M3-08** | M3 全局 Feed 端到端全生命周期与 8 组边界 E2E 测试 | M3-07 | 在 `test/e2e/feed_lifecycle_test.go` 中验证：首页首次加载 → 游标翻页无重无漏 → 连续下架视频自动后移补齐与游标推进断言 → Floor 临界跨越冷热拼接无缝衔接 → 模拟 Redis 宕机平滑降级 MySQL → 游客与登录态软鉴权隔离 → **播放行为上报驱动 `view_count` 自增与近期曝光记录落盘**等核心用例 | `DONE` |

---

## 九、M4 互动、推拉关注流与已赞作品任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M4-01** | 互动、关系与本地消息表迁移脚本 | M0-05 | 编写 `000005_create_interaction_tables`、`000006_create_relation_tables` 与 `000007_create_outbox_tables`；通过 `migrate up` 与 `migrate down` 回滚验证；CI 历史不变性校验绿灯 | `DONE` |
| **M4-02** | 互动、关系与 Outbox 领域模型契约定义 | — | 在 `internal/domain/interaction`、`internal/domain/relation`、`internal/domain/outbox` 定义核心实体（`Like`、`Comment`、`CommentLike`、`Follow`、`RelationStats`、`Friendship`、`OutboxEvent`）、仓储接口契约与领域错误常量；保持零外部依赖 | `DONE` |
| **M4-03** | 视频点赞原子一致性与「我点赞的视频」流实现 | M4-01, M4-02 | 在 `internal/infra/persistence/interaction` 基于 GORM 实现点赞仓储；采用 `INSERT ... ON DUPLICATE KEY UPDATE` 与 `DELETE`，根据 `RowsAffected == 1` 驱动原子加减 `video_stats.like_count`（`GREATEST` 防负数）；单事务内写入 `interaction.liked/unliked` 的 Outbox 记录；实现「我点赞的视频」基于 `likes.id < cursor` 倒序游标分页查询（关联查询有效视频列表）；实现批量点赞探测 `BatchIsLiked` | `DONE` |
| **M4-04** | 评论发布幂等性、回复层级与热评/最新排序实现 | M4-01, M4-02 | 在 `internal/infra/persistence/interaction` 实现评论仓储；基于 `UNIQUE(user_id, idempotency_key)` 唯一键拦截请求重放，命中时幂等返回原有评论；单事务内原子自增 `video_stats.comment_count` 并写 Outbox；支持一级评论与二级单层回复；实现作者删除评论（原子递减 `comment_count`，状态置为已删除）；实现评论点赞/取消点赞与 `comments.like_count` 原子更新；支持评论列表按最新（`id < cursor`）与按热评（`(like_count, id) < (cursor_like_count, cursor_id)`）复合游标分页 | `DONE` |
| **M4-05** | 关注关系、统计维护与互粉好友状态判定 | M4-01, M4-02 | 在 `internal/infra/persistence/relation` 实现关注仓储；自关注校验拦截；单事务内通过 `RowsAffected` 驱动原子维护 `relation_stats`（关注者的 `following_count` 与被关注者的 `follower_count`），同时写入 `relation.followed/unfollowed` Outbox 记录；实现关注列表与粉丝列表单调 ID 游标分页（`follows.id < cursor`）；实现双方关注关系探测与互粉好友判定（`is_friend = is_following && is_follower`）；实现批量关注态探测 `BatchIsFollowed` | `DONE` |
| **M4-06** | Outbox 事务本地消息表与分布式租约调度器 | M4-01, M4-02 | 在 `internal/infra/persistence/outbox` 与 `internal/infra/outbox` 实现调度器：实现抢占式租约领取（生成实例 UUID `lease_token`，原子更新 `status='sending', lease_until=NOW()+30s, attempts=attempts+1`，单次提取 100 条）；实现过期租约自动回收（`status='sending' AND lease_until < NOW()` 重置为 `pending`）；失败时按指数退避计算 `next_retry_at`（1s/5s/25s/2m/10m）；超过 10 次置为 `dead`；提供定时调度循环与优雅退出控制 | `DONE` |
| **M4-07** | RabbitMQ 投递集成与消费端事务去重框架 | M0-06, M4-06 | 封装 RabbitMQ 发布器（开启 Confirm 确认，收到 ACK 后原子更新 Outbox 为 `sent`）；定义交换机 `exchange.gorangefeed.events`（Topic 模式）与持久化队列；实现消费者通用去重中间件/基类：在 MySQL 事务中首行执行 `INSERT INTO consumed_events (event_id) VALUES (?)`，主键冲突直接回滚跳过，保障 At-least-once 投递下的 Exactly-once 语义；注册并挂载事件消费者路由 | `DONE` |
| **M4-08** | 关注流推拉结合（Hybrid Push-Pull）分发与读聚合 | M4-05, M4-07, M3-02 | 1. **写扩散**：Worker 消费 `video.published` 事件，若作者粉丝数 `< 5000`（普通作者），批量分片将 `video_id` 写入所有粉丝 Redis Inbox ZSet `feed:following:inbox:{uid}`（score=video.id，保留最新 800 条裁剪）；若粉丝数 `≥ 5000`（大 V 作者），仅写入作者专有 Outbox ZSet `feed:following:author:{author_id}`；<br>2. **读聚合**：在 `internal/service/feed` 增加 `ListFollowingFeed`，Pipeline 并行读取用户 Inbox ZSet 和关注的所有大 V 的 Outbox ZSet，内存中基于优先队列多路归并排序并游标裁剪；若 Redis 为空或冷启动，平滑降级执行 MySQL 关联查询兜底，并异步触发 Inbox 预热 | `DONE` |
| **M4-09** | 全局 Feed 架构演进：L1 骨架共享与个性化互动态装配解耦 | M3-03, M3-05, M4-03, M4-05 | 重构 `internal/service/feed` 读链路：全局最新流（`scene=global`）将 L1 页缓存升级为跨登录态公共骨架 `feed:page:global:public:{cursor}:{limit}:{rankVersion}`，无论游客还是登录用户全平台共享纯 ID 骨架缓存与 Singleflight 防击穿（缓存复用率从单人孤岛提升至 100%）；将用户个性化状态注入后置到 `hydrateFeedPage` 阶段：若上下文存在 `viewer_id > 0`，并行调用 `BatchIsLiked` 与 `BatchIsFollowed`，动态回填 `item.IsLiked` 与 `item.IsFollowed`，完成纯净骨架与个性化互动态的两阶段解耦 | `DONE` |
| **M4-10** | 历史累计高赞榜单流（`scene=like_rank`） | M3-01, M4-03 | 在 `internal/infra/persistence/feed` 与 `internal/service/feed` 实现历史高赞榜（`scene=like_rank`）；在 MySQL 基于 `(status, like_count DESC, id DESC)` 联合索引进行复合游标分页（`(like_count, id) < (cursor_like_count, cursor_id)`），支持大并发爆款视频榜单查询，与最新流同等享受 L3 卡片与 L4 实时计数缓存体系 | `DONE` |
| **M4-11** | 互动与关系 HTTP 控制器与全路由装配 | M4-03~05, M4-08~10 | 在 `internal/http/interaction`、`internal/http/relation`、`internal/http/feed` 暴露完整 API；统一响应信封与错误码；参数校验与 JWTAuth / SoftJWTAuth 中间件装配；在 `cmd/api/main.go` 中完成全依赖注入；编写 Handler 单元测试 | `DONE` |
| **M4-12** | Worker 服务装配与 Outbox 轮询调度器启动 | M4-06, M4-07, M4-08 | 在 `cmd/worker/main.go` 中装配 MySQL、Redis、RabbitMQ，初始化 Outbox 租约抢占调度器、过期租约回收协程、RabbitMQ 事件发布与消费者订阅管道；接入 `/metrics` 导出 Outbox 积压量、派发成功数、重试数与死信报警指标；接入 SIGTERM 信号优雅停机，确保在途事件消费完毕后安全退出 | `DONE` |
| **M4-13** | M4 端到端互动与推拉流全生命周期 E2E 集成测试 | M4-11, M4-12 | 在 `test/e2e/interaction_lifecycle_test.go` 中编写完整黑盒 E2E 测试集：验证点赞/取消幂等与计数准确性 → 我点赞的视频流倒序游标验证 → 评论幂等防重放与热评排序 → 关注/取关与粉丝统计一致性 → 互粉好友判定断言 → 关注普通作者（写扩散）与关注大 V（读扩散）在关注流中的多路归并排序验证 → 取关后关注流剔除验证 → 全局流 L1 骨架跨登录态共享与个性化点赞/关注态正确注入 → 历史高赞榜复合游标分页测试 | `DONE` |
| **M4-14** | CP-M4 阶段检查点与高并发数据一致性基准自测 | M4-13 | 验证检查点 CP-M4：Worker 抢占租约杀死模拟（验证 30s 租约过期自动回收恢复为 `pending`）；持续失败 10 次流转为 `dead` 状态；执行并发点赞验证：针对单一视频并发 300 点赞/取消点赞，断言事务执行完毕后 `likes` 记录数与 `video_stats.like_count` 绝对一致，计数零丢失，无负数漂移；执行评论并发幂等验证：50ms 内同一用户携带相同 `idempotency_key` 并发 5 次提交，断言仅生成 1 条评论记录，`comment_count` 仅自增 1 | `DONE` |

---

## 十、M4.5 实时热榜与轻量推荐流任务明细（进阶流）

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M4.5-01** | 热榜与推荐领域模型、打分规则与流水线契约定义 | — | 在 `internal/domain/feed` 定义 `HotBucket`、`HotScoreEvent`、`RecallCandidate`、`RecommendationRule`、`AuthorWindow` 实体；定义 `HotTimelineStore`、`ExposureRepository`、`RecallEngine`、`Diversifier` 抽象接口契约与领域错误常量；保持零外部依赖与单元测试覆盖 | `DONE` |
| **M4.5-02** | Redis 滑动分钟桶打分与 as_of 稳定快照聚合 Store 实现 | M0-06, M4.5-01 | 在 `internal/infra/persistence/feed` 实现 `HotTimelineStore`：封装 `ZINCRBY feed:hot:minute:{yyyyMMddHHmm} delta video_id`（TTL 2小时）；实现基于 `as_of = floor(now, 1min)` 的 60 个分钟桶 `ZUNIONSTORE feed:hot:window:{as_of}` 聚合（TTL 2分钟），并自动执行 `ZREMRANGEBYSCORE dest -inf 0` 剔除零分/负分；实现按 `offset` 的 `ZREVRANGE` 快照翻页 | `DONE` |
| **M4.5-03** | RabbitMQ 热度打分消费者与 Worker 异步管道集成 | M4-07, M4.5-02 | 在 `internal/infra/rabbitmq` 或 `internal/service/feed` 实现 `HotScoreConsumer`；在 `cmd/worker/main.go` 中挂载并订阅 `interaction.*` 与 `video.viewed` 事件；按打分矩阵异步累加分钟桶分数；依托消费端去重保证幂等，无死循环重试 | `DONE` |
| **M4.5-04** | 轻量多路召回引擎（MultiSourceRecallEngine）实现 | M3-02, M4.5-02 | 在 `internal/service/feed` 实现 `MultiSourceRecallEngine`：支持热门高质池（50% 从 Hot Window 提取）、最新时效池（30% 从 Global Timeline 提取）、探索随机池（20% 随机偏移采样）；各路并发获取并合并去重，候选不足时自动平滑互补 | `DONE` |
| **M4.5-05** | 近期曝光过滤与同作者滑动窗口打散器实现 | M0-06, M4.5-01 | 在 `internal/infra/persistence/feed` 与 `internal/service/feed` 实现：<br>1. **曝光过滤器**：基于 Redis Set `reco:exposed:{uid}`（TTL 72h），通过 `Pipeline SMISMEMBER` 高速剔除登录用户近期已看视频；<br>2. **同作者打散器**：实现滑动窗口打散算法，保证单页输出中同一作者视频间隔 ≥ 3 条；候选不足时安全降级 | `DONE` |
| **M4.5-06** | Feed 业务层热榜与推荐编排（ListHotFeed & ListRecommendFeed） | M4.5-02~05, M4-09 | 在 `internal/service/feed` 实现 `ListHotFeed` 与 `ListRecommendFeed`：Hot 流支持 `as_of + offset` 稳定快照翻页；Recommend 流支持多路召回+曝光去重+作者打散；统一步入 L3 视频卡片 MGET + L4 计数 MGET + Dynamic Hydration 个性化装配管道；软鉴权兼容游客与登录态；冷启动与 Redis 异常平滑回源 MySQL | `DONE` |
| **M4.5-07** | HTTP 控制器、透明游标编解码与全路由装配 | M4.5-06 | 在 `internal/http/feed` 增加 `GET /api/v1/feed/hot` 与 `GET /api/v1/feed/recommend`；实现透明 Base64 JSON 游标编解码（内部封装 `scene`, `as_of`, `offset`, `page`, `seed` 等）；挂载 `SoftJWTAuth` 软鉴权中间件；在 `cmd/api/main.go` 中完成全依赖组装与路由注册；编写 Handler 单元测试 | `DONE` |
| **M4.5-08** | M4.5 实时热榜与推荐流全生命周期 E2E 集成测试 | M4.5-07 | 在 `test/e2e/hot_recommend_lifecycle_test.go` 中实现完整黑盒 E2E 测试集：验证互动事件驱动分钟桶热度累加与取消点赞扣分 → as_of 稳定快照分页无重无漏 → 播放行为沉淀曝光记录与二次推荐已看过滤 → 同一作者连续作品强制打散断言 → 游客（未登录）冷启动平滑降级断言 → `make vet && make test` 全量绿灯回归 | `DONE` |

---

## 十二、M5 系统通知与分布式跨实例 SSE 任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M5-01** | 系统通知数据表迁移与复合索引设计 | M0-05 | 编写 `000009_create_notifications_table` 迁移脚本；包含 `id`, `recipient_id`, `actor_id`, `type`, `target_type`, `target_id`, `title`, `content`, `event_id`, `is_read`, `created_at`, `read_at`；建立 `UNIQUE(recipient_id, event_id)` 幂等消费防重约束、`(recipient_id, created_at DESC, id DESC)` 倒序翻页索引、`(recipient_id, is_read)` 未读计数覆盖索引；通过 migrate up/down 回滚验证 | `DONE` |
| **M5-02** | 通知领域模型、枚举与仓储契约定义 | — | 在 `internal/domain/notification` 定义 `Notification` 聚合根（包含 `MarkAsRead()` 状态机流转与富文本截断规则）、`NotificationType` 与 `TargetType` 枚举、`NotificationCursor` 游标值对象、`Repository` 接口契约与领域错误常量；保持零外部依赖与单元测试覆盖 | `DONE` |
| **M5-03** | 通知仓储持久化与覆盖索引查询实现 | M5-01, M5-02 | 在 `internal/infra/persistence/notification` 基于 GORM 实现 `Repository`；实现基于 `(created_at, id) < (cursor_created_at, cursor_id)` 的复合游标翻页查询与动态 Actor 快照回填；实现基于 `(recipient_id, is_read)` 覆盖索引的高性能未读计数统计；实现单条与一键批量标记已读（原子更新 `is_read=true, read_at=NOW()`）；实现断线补拉查询 `ListAfter` | `DONE` |
| **M5-04** | 评论 `@提及` 用户解析器与通知服务 | M5-02 | 在 `internal/service/notification` 实现提及解析器：正则提取评论中的 `@username`（去重并限制上限 10 人）；联动 `accountRepo` 批量查询目标用户 ID；过滤自提及与视频作者自身；批量生成 `mention` 类型系统通知并持久化落库；覆盖单测 | `DONE` |
| **M5-05** | 分布式事件广播器抽象与 Redis Pub/Sub 引擎实现 | M0-06 | 抽象通用 `Broadcaster` 接口（`Publish`, `Subscribe`, `Close`）；在 `internal/infra/redis` 实现 `RedisBroadcaster`，订阅集中式广播主题 `channel.gorangefeed.notifications`；提供 `MemoryBroadcaster` 供本地单机与轻量单测使用；支持断线自动重试与重新订阅 | `DONE` |
| **M5-06** | RabbitMQ 通知消费者与跨进程 Redis Pub/Sub 事件广播 | M4-07, M5-03~05 | 在 `internal/service/notification` 实现 `NotificationConsumer`：订阅 `interaction.liked`、`interaction.commented`、`relation.followed` 等事件；过滤自身操作防自扰；关联视频/评论作者并触发落库与 `@mention` 派发；依托消费端去重表与唯一键保障幂等；落库成功后调用 `Broadcaster.Publish` 向 Redis 广播信令；在 `cmd/worker/main.go` 中挂载并启动 | `DONE` |
| **M5-07** | 分布式 SSE Hub 连接池、短期 Ticket 安全鉴权与 60s 重叠窗口断线补拉引擎 | M0-06, M5-02, M5-03, M5-05 | 在 `internal/service/notification` 实现 `SSEHub`：维护内存活跃客户端连接池（通道缓冲 64，溢出主动剔除断开）；常驻协程监听 Redis 广播频道并在本地内存 `clients[uid]` 执行 O(1) 匹配投递；实现 15s 定时心跳注释（`: keepalive\n\n`）；实现基于 Redis 的短期一次性 Ticket 交换与安全销毁（`sse:ticket:{uuid} -> uid`, EX 60s，Lua 原子消费）；实现 `Last-Event-ID` 解析与 60s 重叠窗口回退查询，在连接建立时优先补推遗漏历史通知再接入实时通道 | `DONE` |
| **M5-08** | 节点停机平滑 Drain、在线状态心跳与 Prometheus 大盘指标 | M5-07 | 维护节点标识 `node_id`（主机名+随机 UUID 码）；通过 Redis Hash 定期上报活跃长连接数（TTL 30s）；导出 Prometheus 指标：`sse_connected_clients`、`sse_broadcast_received_total`、`sse_broadcast_sent_total`、`sse_dispatch_latency_seconds`；实现优雅退出协同：API 实例接收 SIGTERM 信号时，先向所有本地活跃 SSE 客户端推送协议级注释行通知 `: server_restarting\n\n` 并平滑关闭长连接，引导客户端自动漂移重连存活节点 | `DONE` |
| **M5-09** | 通知 HTTP 控制器、SSE 流路由与全路由装配 | M5-06~08 | 在 `internal/http/notification` 暴露 `GET /api/v1/notifications`、`GET /api/v1/notifications/unread-count`、`PATCH /api/v1/notifications/:id/read`、`POST /api/v1/notifications/read-all`、`POST /api/v1/notifications/sse/ticket`、`GET /api/v1/notifications/stream`；统一响应信封与错误码；绑定 JWTAuth 与 Ticket 鉴权；在 `cmd/api/main.go` 完成依赖注入与路由挂载；编写 Handler 单测 | `DONE` |
| **M5-10** | M5 端到端通知闭环、断线补拉与多实例集群广播全生命周期 E2E 集成测试 | M5-09 | 在 `test/e2e/notification_lifecycle_test.go` 中实现完整黑盒 E2E 测试集：验证点赞/评论/关注驱动通知异步落库与自操作过滤 → 评论 `@提及` 用户生成 mention 通知 → 未读计数与单条/一键已读原子流转 → 通知列表复合游标无重无漏翻页 → SSE Ticket 安全置换与防二次使用 → 模拟用户在线 SSE 实时接收通知推送（端到端 < 100ms） → 模拟断线 10s 期间产生新通知并携带 Last-Event-ID 完整补拉无遗漏 → 启动双节点模拟跨节点 Redis 广播到达与节点停机优雅漂移接管 → `make vet && make test` 全量绿灯回归 | `DONE` |

---

## 十三、M6 性能压测与播放 QoS 体验监控任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M6-01** | 播放体验与 QoS 领域模型、Prometheus 指标与契约定义 | — | 在 `internal/domain/playback` 定义 `PlaybackConfig`、`PlaybackQualityEvent`、`NetworkType` 实体与枚举；在 `internal/service/playback` 定义 Prometheus 埋点指标（首帧耗时直方图、卡顿次数/时长计数器、完播与会话计数器）与抽象服务接口契约；保持零外部依赖与单元测试覆盖 | `DONE` |
| **M6-02** | 播放预加载策略推荐器与 QoS 指标聚合服务 | M6-01 | 在 `internal/service/playback` 实现 `Service`：基于客户端网络环境（WiFi/5G/4G/弱网）自适应决策滑动预加载条数（1~3条）、首帧缓冲阈值（300ms）与最大缓冲阈值（5000ms）；批量聚合处理端侧上报的播放 QoS 质量事件并驱动 Prometheus 观测大盘数据更新；单元测试覆盖 | `DONE` |
| **M6-03** | 播放 HTTP 控制器、路由挂载与全依赖装配 | M6-02 | 在 `internal/http/playback` 暴露 `GET /api/v1/playback/config` 与 `POST /api/v1/playback/events`；支持 SoftJWTAuth 软鉴权；在 `cmd/api/main.go` 中完成全依赖组装与路由注册；编写 Handler 单元测试 | `DONE` |
| **M6-04** | 核心读链路（最新流/推荐流/热榜流）高并发性能基准压测套件 | M3, M4.5 | 在 `test/benchmark/feed_benchmark_test.go` 中编写基于 Go 原生 Benchmark 的压测套件：模拟高并发压测最新流 L1 骨架共享（1.31ms）、动态水化（3.61ms）、滑动窗口分钟桶聚合（2.61ms）与推荐流多路召回曝光打散（16.59ms） | `DONE` |
| **M6-05** | 核心写链路（高并发点赞行锁与 Outbox 事务调度）性能基准压测套件 | M4, M5 | 在 `test/benchmark/interaction_benchmark_test.go` 中编写压测套件：单视频热点并发点赞压测（验证垂直拆分 `video_stats` 行锁与 Outbox 同事务落盘耗时仅 13.68ms）；压测 Worker 抢占式分布式租约调度与批量投递吞吐 | `DONE` |
| **M6-06** | M6 端到端 QoS 生命周期验证与 Benchmark 综合分析报告 | M6-03~05 | 在 `test/e2e/playback_lifecycle_test.go` 中验证自适应预加载配置下发、播放卡顿与首帧上报及 Prometheus `/metrics` 真实指标增长；执行全量 Benchmark 并将压测性能指标沉淀为 `docs/BENCHMARK_REPORT.md`；`make vet && make test` 全量绿灯回归 | `DONE` |

---

## 十四、M7 创作中心与高可靠发布时序任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M7-01** | 创作中心数据表迁移与 `publish_seq` 扩展 | M0-05, M2-01 | 编写 `000010_create_creator_and_publish_seq_tables` 迁移脚本；创建 `drafts` 表（`id, author_id, title, description, play_url, cover_url, duration_ms, status, created_at, updated_at`）与 `(author_id, updated_at DESC, id DESC)` 索引；创建 `scheduled_tasks` 表；在 `videos` 表增加 `publish_seq BIGINT NOT NULL DEFAULT 0` 字段与唯一复合索引 `UNIQUE KEY uk_publish_seq (publish_seq)`；通过 migrate up/down 回滚验证 | `DONE` |
| **M7-02** | 创作中心领域模型与分布式发号器契约定义 | — | 在 `internal/domain/creator` 定义 `Draft` 聚合根、`DraftStatus` 枚举、`ScheduledTask` 实体、`ScheduledStatus` 枚举、`DraftRepository`、`ScheduledTaskRepository` 契约及 `SequenceGenerator` 分布式发号器接口；零外部依赖与单元测试覆盖 | `DONE` |
| **M7-03** | 草稿箱持久化仓储与游标翻页实现 | M7-01, M7-02 | 在 `internal/infra/persistence/creator` 基于 GORM 实现 `DraftRepository`；实现草稿存、取、改、删与作者数据隔离；实现基于 `(updated_at, id)` 复合游标的分页拉取；实现草稿向正式视频一键发布的原子流转 | `DONE` |
| **M7-04** | 分布式发号器与时间线时序空洞消除机制 | M0-06, M7-01, M7-02 | 在 `internal/infra/redis` 实现基于 Redis `INCR seq:video:published` 的分布式发号器；改造 Outbox 发布消费端，在视频正式发布瞬间原子申请连续递增的 `publish_seq` 并回写 `videos`；全局时间线 ZSet score 切换为 `publish_seq`，彻底根治长事务提交乱序与延迟发布导致的翻页漏看空洞 | `DONE` |
| **M7-05** | 定时发布延迟队列与分布式 Worker 抢占调度器 | M0-06, M7-01, M7-02 | 在 `internal/infra/scheduler` 实现基于 Redis ZSet 延迟队列（score 为计划发布 Unix 时间戳）与分布式锁的定时发布调度器；计划到期秒级出队、CAS 状态防重流转、入队 Outbox 触发原子发号发布流程；支持故障重试与节点竞争互斥 | `DONE` |
| **M7-06** | 创作中心业务服务编排与发布状态机驱动 | M7-03~05 | 在 `internal/service/creator` 实现 `Service`：草稿保存与校验、定时发布计划时间校验（限制未来 5 分钟至 30 天以内）、定时任务取消并退回草稿状态机、联动 `videoRepo` 正式发版；单元测试覆盖 | `DONE` |
| **M7-07** | 创作中心 HTTP 控制器与全路由依赖装配 | M7-06 | 在 `internal/http/creator` 暴露 `POST/GET/PUT/DELETE /api/v1/creator/drafts`、`POST /api/v1/creator/publish-scheduled`、`GET/DELETE /api/v1/creator/scheduled-tasks`；统一响应信封与错误码；在 `cmd/api/main.go` 与 `cmd/worker/main.go` 完成依赖注入与路由/调度引擎挂载；编写 Handler 单测 | `DONE` |
| **M7-08** | M7 创作中心与高可靠发布时序全生命周期 E2E 集成测试 | M7-07 | 在 `test/e2e/creator_lifecycle_test.go` 中实现完整黑盒 E2E 测试集：验证草稿增删改查 → 草稿一键发布 → 定时发布排期时间校验拦截 → 定时到期秒级准点自动触发入时间线 → 取消定时发布并回退草稿 → 50 并发发布单调发号严格递增验证 → 乱序提交下的时间线无漏翻页断言 → 并发 Worker 抢占防重复调度断言 → `make vet && make test` 全量绿灯回归 | `DONE` |

---

## 十五、M8 视频收藏夹与播放历史资产中心任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M8-01** | 收藏夹与播放历史数据表迁移 | M0-05 | 编写 `000011_create_favorites_and_history_tables` 迁移脚本；创建 `favorites`（收藏夹元数据）、`favorite_items`（收藏关系表，`UNIQUE(favorite_id, video_id)`）、`watch_history`（历史观看记录表）；在 `video_stats` 增加 `favorite_count` 字段；通过 migrate up/down 验证 | `TODO` |
| **M8-02** | 收藏与历史领域模型与仓储契约定义 | — | 在 `internal/domain/asset` 定义 `FavoriteFolder` 聚合根、`FavoriteItem`、`WatchHistory` 实体、滑动窗口保留策略值对象与仓储契约；单元测试覆盖 | `TODO` |
| **M8-03** | 收藏夹与播放历史持久化仓储实现 | M8-01, M8-02 | 在 `internal/infra/persistence/asset` 实现仓储；基于 Redis ZSet 实现单个用户最近 100 条播放历史毫秒级滑动维护，异步落库 MySQL；实现默认收藏夹自动初始化与多收藏夹归类；原子同步自增/自减 `video_stats.favorite_count` | `TODO` |
| **M8-04** | 资产中心服务编排与热度推荐正向加权 | M8-03, M4.5 | 在 `internal/service/asset` 实现业务服务：收藏/取消收藏、历史进度记录与断点续播查询；触发 `interaction.favorited` 事件，在热榜滑动分钟桶给予 3 倍于点赞的高权重热度赋能；单元测试覆盖 | `TODO` |
| **M8-05** | 资产中心 HTTP 控制器与全路由挂载 | M8-04 | 在 `internal/http/asset` 暴露 `POST/DELETE /api/v1/videos/:id/favorite`、`GET/POST/DELETE /api/v1/user/favorites`、`GET /api/v1/user/history`；全依赖组装与路由注册；Handler 单测 | `TODO` |
| **M8-06** | M8 资产中心全生命周期 E2E 集成测试 | M8-05 | 在 `test/e2e/asset_lifecycle_test.go` 中验证黑盒测试：收藏与取消收藏原子计数流转 → 默认收藏夹与自定义收藏夹分类管理 → 跨设备断点续播进度保存与历史滑动窗口修剪 → 收藏行为驱动热榜热度倍增与推荐流正向加权断言 → `make vet && make test` 全量绿灯回归 | `TODO` |

---

## 十六、M9 内容合规与安全审核中台任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M9-01** | 审核任务与违规治理数据表迁移 | M0-05 | 编写 `000012_create_moderation_tables` 迁移脚本；创建 `review_tasks` 表（`video_id, status, risk_score, matched_words, reviewer_id, review_note`）与 `moderation_logs` 表；通过 migrate up/down 回滚验证 | `TODO` |
| **M9-02** | 审核中台领域模型与规则引擎契约定义 | — | 在 `internal/domain/moderation` 定义 `ReviewTask` 聚合根（包含 `Approved/Rejected/Pending` 状态机流转）、`ReviewPolicy` 策略枚举（先审后发/先发后审）、`SensitiveMatcher` 接口契约；单元测试覆盖 | `TODO` |
| **M9-03** | 基于 Aho-Corasick 算法的敏感词机审引擎 | M9-02 | 在 `internal/service/moderation` 实现高性能敏感词检测器：构建双数组 AC 自动机，支持 10 万+ 敏感词毫秒级多模式匹配，命中敏感词自动打标并根据风险权重自动判定拦截；单元测试覆盖 | `TODO` |
| **M9-04** | 运营管理后台控制器与全路由挂载 | M9-03 | 在 `internal/http/admin` 暴露 `GET /api/v1/admin/reviews`、`POST /api/v1/admin/videos/:id/review`、`POST /api/v1/admin/videos/:id/takedown`、`POST /api/v1/admin/users/:id/ban`；绑定管理员 RBAC 权限校验；依赖装配与 Handler 单测 | `TODO` |
| **M9-05** | M9 内容审核与合规治理全生命周期 E2E 集成测试 | M9-04 | 在 `test/e2e/moderation_lifecycle_test.go` 中验证黑盒测试：敏感词自动命中打标与风险阻断 → 先发后审与先审后发双轨流转 → 运营审核驳回自动下架视频与撤销时间线索引 → 违规账号封禁后鉴权即时失效 → `make vet && make test` 全量绿灯回归 | `TODO` |

---

## 十七、M10 全栈 Web 短视频流式演示客户端任务明细

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M10-01**| 响应式短视频流 Web 播放器脚手架 | M2, M3 | 在 `web/` 下构建响应式 HTML5 极简短视频播放界面（无厚重前端构建依赖）；支持鼠标滚轮与移动端触屏上下滑切视频；集成 HTTP 206 Partial Content 分段流式解码播放与首屏静音自动起播 | `TODO` |
| **M10-02**| 互动交互动效与实时 SSE 弹窗集成 | M4, M5, M10-01 | 接入点赞/关注双向绑定；双击屏幕触发红心飞出动画；接入 `/api/v1/notifications/stream` SSE 长连接，实现新点赞/新评论右上角 Toast 浮窗与未读红点毫秒级闪烁 | `TODO` |
| **M10-03**| 端侧播放 QoS 实时观测大盘看板 | M6, M10-02 | 自动采集起播首帧耗时、卡顿事件与播放时长并向 `/api/v1/playback/events` 自动上报；在前端悬浮抽屉中展示实时 QoS 看板（当前预加载策略、卡顿率、首帧耗时直方图） | `TODO` |





