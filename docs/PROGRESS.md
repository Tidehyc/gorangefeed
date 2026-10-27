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
| **当前阶段** | **M3 全局最新 Feed 流任务拆解已就绪，准备开启编码落地（TODO）** |
| **完成进度** | M0 阶段：15 / 15 项（100%） · M1 阶段：14 / 14 项（100%） · M2 阶段：9 / 9 项（100%） · M2.5 阶段：7 / 7 项（100%） · M3 阶段：0 / 8 项（0%） |
| **进行中任务** | 已完成 M3 全局 Feed 架构骨架设计与 8 项任务细分（领域模型、Redis ZSet 时间线与 Lua 裁剪、多级缓存契约、MySQL 冷区兜底、Service 降级编排、HTTP 控制器与端到端测试方案） |
| **阻塞事项** | 无 |
| **下一步行动** | 待用户确认后，按序开展 M3-01 领域模型与契约定义及后续研发 |

---

## 三、里程碑总览

| 里程碑 | 主题 | 关联需求 | 状态 | 核心完成标准 |
|---|---|---|---|---|
| **M0** | 工程骨架与基础设施装配 | NFR-MAINT-*, NFR-OBS-01 | `DONE` | `go vet` + `go test` 绿灯；`docker compose` 一键启动；`/readyz` 正确反映依赖状态，`/metrics` 输出标准指标 |
| **M1** | 账号、双 Token 与安全中间件 | FR-ACC-01~06, NFR-SEC-07 | `DONE` | 登录、刷新、登出及旧 Token 撤销全流程通过；软鉴权与频控限流测试通过 |
| **M2** | 视频单文件上传与流式发布 | FR-VID-01~05, NFR-EXT-01 | `DONE` | E2E 单测跑通上传、发布、206 Range 分段点播、详情、游标分页与下架；`videos` 与 `video_stats` 事务一致 |
| **M2.5**| 分片上传与断点续传（二期） | FR-VID-06~07 | `DONE` | 200MB 文件断网重试续传成功；基于 Redis Bitmap 与分布式锁杜绝并发脏写与重复合并；MySQL 权威状态机保证一致性 |
| **M3** | 全局最新流与播放曝光上报 | FR-FEED-01, 03, 04, FR-VID-08 | `TODO` | 游标翻页无重无漏；Redis 异常时自动平滑降级 DB；下架内容补页正确；上报播放行为驱动 `view_count` 自增与曝光打底 |
| **M4** | 互动、推拉关注流与已赞作品 | FR-INT-*, FR-REL-*, FR-FEED-02, 07 | `TODO` | 高并发点赞计数不丢；我点赞的视频流倒序分页；评论支持幂等防重放与点赞；关注流推拉合并正确；互粉好友判定；Outbox 运转正常 |
| **M4.5**| 实时热榜与轻量推荐流（进阶流） | FR-FEED-05, FR-FEED-06 | `TODO` | 60分钟滑动窗口互动热度衰减（ZSet分钟桶）；as_of+offset稳定快照分页；热门/最新/探索多路召回、曝光去重与同作者打散 |
| **M5** | 系统通知与单实例 SSE | FR-NOT-01~05, FR-NOT-07 | `TODO` | 点赞/评论/关注/提及通知 P99 < 1s 异步落库；SSE 在线推送与断线补拉验证完整 |
| **M5.5**| 跨实例 SSE 广播（二期） | FR-NOT-06 | `TODO` | 多实例下 Pub/Sub 广播到达；断连期间通知通过补拉恢复 |
| **M6** | 性能压测与播放 QoS 体验监控 | NFR-PERF-*, FR-QOS-* | `TODO` | 核心链路达到性能设计指标或产出基准分析报告；端侧预加载策略建议与播放卡顿/首帧 QoS 上报监控 |

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

### 8.1 参考项目对比与架构优化决策（取长补短）

对比两个成熟参考项目（`feedsystem_video_go` 与 `GCFeed`），分析其在全局 Feed 流方案上的长处与短板，确立 GoRangeFeed 的针对性优化策略：

| 维度 | `feedsystem_video_go` 实现 | `GCFeed` 实现 | GoRangeFeed 优化决策（取长补短） |
|---|---|---|---|
| **排序权威源与游标设计** | 使用发布毫秒戳 `CreateTime.UnixMilli()` 作为 ZSet score 与游标；存在**同毫秒多视频并发写入时的碰撞丢失与重叠问题**。 | 使用 `published_at + video_id` 复合游标编码为 Base64；无 ZSet 时间线，依赖 DB 索引扫描。 | **单调 ID 统一游标**：严格利用自增主键 `video.id`（单调递增）作为 score 与 cursor（`score = video.id`），天然杜绝毫秒碰撞、时区漂移与浮点数精度损耗，前端透明易解析。 |
| **滑动窗口与边界维护** | 每次读请求执行 `ZRangeWithScores(0, 0)` 获取尾部水印，存在**高频额外 RTT**；且无原子裁剪。 | 无滑动窗口 ZSet。 | **Lua 原子裁剪 + 独立 Floor 缓存**：通过后台任务计算 72h Floor，采用 Lua 脚本原子执行 `ZREMRANGEBYSCORE` 裁剪与 `SET timeline:floor`，读路径直接 `GET timeline:floor`，杜绝冗余 `ZRange` 开销。 |
| **冷热无缝拼接 (Stitching)** | **【优点】** 当 ZSet 热区数据不足 `limit` 时，计算 `remainLimit = limit - len(hotVideos)` 并回源 DB 拼接冷区。 | 无冷热拼接机制，全量走 DB 过滤或页缓存。 | **【吸纳长处】** 完整采纳并规范化冷热拼接算法：热区扫描不足时，以最后扫描到的冷区边界 ID 为游标无缝向 MySQL 补齐差额数据，保证每页数量对齐。 |
| **已下架/软删除视频处理** | **【短板】** 遇到非发布状态仅在内存中剔除，游标依然停留在有效条目，导致后续翻页**反复空转扫描已删视频**，且返回条数不足。 | 依赖 DB 联合索引过滤 `status = published`。 | **【弥补短板】推进扫描指针（`NextCursor = lastScannedID`）**：批量探测 `max(limit*2, 50)`，遇到已下架内容自动后移补齐（单次扫描上限 500 条），且无论是否有效，游标均推至最后探测的 VideoID，彻底根治重复扫描。 |
| **缓存分层架构** | 仅使用本地缓存（go-cache）+ Redis 缓存单实体视频，缺少轻量页缓存，高并发下所有请求仍穿透 ZSet。 | **【优点】** 采用工业级 L1 页缓存（5s/45s）+ L3 卡片（15m）+ L4 计数（15s），按实体与动态计数分离。 | **【吸纳长处并增强】** 采纳 L1/L3/L4 分层模型，同时引入 **逻辑过期（Logical Expiration）+ Singleflight 进程内防击穿**：未命中时单协程回源，过期在 30s 容忍窗口内快速返回旧值并在后台异步刷新，保障极低 P99 延迟。 |
| **鉴权与上下文注入** | 登录态与游客态隔离，但缺少规范化软鉴权中间件。 | 接口允许匿名访问。 | **标准 `SoftJWTAuth` 软鉴权**：同时兼容 Header 与 URL Query `?token=`，匿名请求返回通用公共流，登录用户自动注入上下文（为 M4 点赞/关注态打下基础）。 |

---

### 8.2 M3 任务明细拆解

| 编号 | 任务描述 | 前置依赖 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M3-01** | Feed 领域模型、游标分页对象与仓储契约定义 | — | 在 `internal/domain/feed` 定义 `FeedItem`、`FeedPage`、`FeedCursor`、`CachedPage`（逻辑过期模型）、`ViewEvent`（播放与曝光实体）、`TimelineRepository`、`PageCacheRepository`、`CardCacheRepository`、`FeedDBRepository`、`ExposureRepository` 接口及领域错误常量，保持零外部依赖与单元测试覆盖 | `TODO` |
| **M3-02** | Redis ZSet 全局时间线与 Lua 原子裁剪落地 | M0-06, M3-01 | 在 `internal/infra/persistence/feed` 实现 `TimelineStore`；封装 `timeline:global`（score=video.id）与独立 `timeline:floor`；基于 Lua 脚本原子执行 `ZREMRANGEBYSCORE` 裁剪与 Floor 更新；支持 `ZREVRANGEBYSCORE (cursor -inf LIMIT 0 N` 倒序游标查询与冷启动重建 | `TODO` |
| **M3-03** | 多级缓存体系（L1 逻辑过期+Singleflight、L3 卡片、L4 实时计数）实现 | M0-06, M3-01 | 采纳 GCFeed 模式实现 L1 页缓存（`feed:page:{scene}:{viewer}:{cursor}:{limit}:{rankVersion}`，含逻辑过期结构体与 `singleflight.Group` 进程内防击穿并发合并，30s 容忍平滑返回）；实现 L3 视频卡片（`feed:card:{videoID}`，15m TTL）与 L4 统计缓存（`feed:stat:{videoID}`，15s TTL）的批量 MGET/MSET 存取 | `TODO` |
| **M3-04** | MySQL 冷数据兜底查询与 Floor 动态分界持久化 | M2-01, M3-01 | 在 `internal/infra/persistence/feed` 基于 GORM 实现 `FeedDBRepository`；实现基于 `idx_status_id (status, id DESC)` 的单调 ID 游标翻页查询（`id < cursor`）；实现 72h Floor 动态分界计算 `SELECT MIN(id) FROM videos WHERE status = 1 AND published_at >= NOW() - INTERVAL 72 HOUR` | `TODO` |
| **M3-05** | Feed 业务编排、冷热拼接与播放曝光上报闭环 | M3-01~04 | 在 `internal/service/feed` 编排读链路：L1 命中返回 → L2 时间线拉取 candidate IDs → 下架状态过滤与 `next_cursor` 推进至最后扫描项（单次扫描上限 500 条） → **冷热无缝拼接（热区不足时计算 remainLimit 从 MySQL 补齐）** → 越过 Floor 或 Redis 异常自动平滑降级 MySQL 冷区 → L3/L4 缺失异步批量回填 → 回写 L1 页缓存；**实现播放与曝光行为处理（驱动 `video_stats.view_count` 原子累加，异步写入 Redis 近期用户曝光集合为推荐去重打底）**；提供视频发布后的时间线追加钩子；覆盖单测与 Mock 测试 | `TODO` |
| **M3-06** | Feed HTTP 控制器与软鉴权路由挂载 | M3-05 | 在 `internal/http/feed` 暴露 `GET /api/v1/feed/global` 全局流接口与 `POST /api/v1/videos/:videoId/view` 播放曝光上报接口；挂载 `SoftJWTAuth` 中间件（匿名返回通用流，登录态解析 `userID` 注入上下文）；支持 `cursor` 与 `limit`（默认 10，最大 30）参数校验；统一 API Envelope；编写 Handler 单元测试 | `TODO` |
| **M3-07** | 路由挂载与依赖装配联动 | M3-06 | 在 `cmd/api/main.go` 完成 Feed 仓储、多级缓存、Service 与 Handler 的依赖组装；注册 `/api/v1/feed/*` 与 `/api/v1/videos/:videoId/view` 路由；在视频发布成功后联动触发时间线同步更新（写失效记录 Warn 日志不阻断发布）；探针检查生效 | `TODO` |
| **M3-08** | M3 全局 Feed 端到端全生命周期与 8 组边界 E2E 测试 | M3-07 | 在 `test/e2e/feed_lifecycle_test.go` 中验证：首页首次加载 → 游标翻页无重无漏 → 连续下架视频自动后移补齐与游标推进断言 → Floor 临界跨越冷热拼接无缝衔接 → 模拟 Redis 宕机平滑降级 MySQL → 游客与登录态软鉴权隔离 → **播放行为上报驱动 `view_count` 自增与近期曝光记录落盘**等核心用例 | `TODO` |

---

## 九、后续里程碑任务明细规划

- **M4 互动、推拉关注流与已赞作品**：
  - 点赞/关注表迁移与数据一致性（点赞与 `video_stats.like_count` 事务一致，`RowsAffected` 驱动原子防重复）；
  - **当前用户已赞作品列表（`GET /api/v1/users/me/likes`，按 `likes.created_at DESC` 游标分页）**；
  - 评论表（支持基于客户端 `idempotency_key` 唯一键防重放、一级评论与二级单层回复）、**评论点赞与热评按赞数排序**；
  - 关注关系表与关系统计、**互粉好友状态判定（`is_friend=true`）**、**历史累计高赞榜单（点赞榜 `like_count DESC, id DESC` 复合游标）**；
  - Outbox 本地消息表事务落库与租约派发调度器（抢占式领取、超时回收、指数退避、死信流转）；
  - 消费端事务幂等（`consumed_events`）；
  - 关注流推拉结合（普通作者写扩散至粉丝 Redis Inbox，大 V 作者专有 Outbox，读路径多路归并）。
- **M4.5 实时热榜与轻量推荐流（进阶流）**：
  - **实时滑动热榜（`scene=hot`）**：M4 互动事件（点赞/评论）与 M3 播放事件通过 Outbox 异步打分写入 Redis 60 个 1 分钟桶（`feed:hot:minute:{yyyyMMddHHmm}`）；基于 `as_of = floor(now, 1min)` 快照执行 `ZUNIONSTORE` 聚合前 60 个桶；客户端携带 `as_of + offset` 稳定快照翻页，彻底根治变分下的漏刷与滑重；
  - **轻量多路召回推荐流（`scene=recommend`）**：三路候选召回（热门高质池 50% + 最新时效池 30% + 随机探索池 20%）；基于 M3 沉淀的 Redis 用户曝光历史（`reco:exposed:{uid}`）执行 72h 近期已看过滤；单页强制同作者间隔打散（间隔 ≥ 3 条）。
- **M5 系统通知与单实例 SSE**：
  - 系统通知明细表（`notifications`）与复合索引设计；
  - Outbox 异步消费生成点赞/评论/关注通知，自身操作不自通知；
  - **评论解析 `@username` 异步派发提及通知（Mention Notification）**；
  - 未读通知计数与游标列表查询、单条/一键已读；
  - SSE Hub 在线推送、短期 Ticket 安全鉴权（防 URL Query 泄露长期 Token）、60s 重叠窗口基于 `Last-Event-ID` 断线补拉。
- **M5.5 跨实例 SSE 广播（二期）**：
  - 基于 Redis Pub/Sub（频道 `notify:{userID}`）实现多 API 实例间的跨节点广播分发；
  - 弱网波动靠 60s 窗口补拉兜底。
- **M6 性能压测与播放 QoS 体验监控**：
  - 核心读写链路（最新流、热榜流、并发点赞、Outbox 吞吐）万级 QPS 极限压测，产出 Benchmark 分析报告；
  - **端侧播放体验支持**：下发端侧滑动预加载建议条数与缓冲阈值（`/api/v1/playback/config`）；收集播放卡顿率、首帧耗时与完播率上报，导出 Prometheus 监控大盘。
- **后续二期进阶演进（M7+）**：
  - 视频服务端 ffmpeg 自动截取第一帧封面与异步多码率转码（HLS / DASH）；
  - 内容安全审核流水线（发布进入待审态 `review_tasks`，机器初审评分 + 人工判定）；
  - 管理员违规强制下架与用户封号后台接口；
  - 视频收藏夹（Favorite）与用户点对点私信系统（DM）。

