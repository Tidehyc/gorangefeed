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
| **当前阶段** | **M0 骨架（已完成）** |
| **完成进度** | M0 阶段：15 / 15 项（100%） |
| **进行中任务** | 无 |
| **阻塞事项** | 无 |
| **下一步行动** | 启动 **M1**：账号域数据模型、密码哈希与双 Token 鉴权中间件 |

---

## 三、里程碑总览

| 里程碑 | 主题 | 关联需求 | 状态 | 核心完成标准 |
|---|---|---|---|---|
| **M0** | 工程骨架与基础设施装配 | NFR-MAINT-*, NFR-OBS-01 | `DONE` | `go vet` + `go test` 绿灯；`docker compose` 一键启动；`/readyz` 正确反映依赖状态，`/metrics` 输出标准指标 |
| **M1** | 账号、双 Token 与安全中间件 | FR-ACC-01~06, NFR-SEC-07 | `TODO` | 登录、刷新、登出及旧 Token 撤销全流程通过；软鉴权与频控限流测试通过 |
| **M2** | 视频单文件上传与流式发布 | FR-VID-01~05, NFR-EXT-01 | `TODO` | `e2e.sh` 跑通上传、发布与详情；视频路由支持 HTTP 206 Range 分段请求；`videos` 与 `video_stats` 数据一致 |
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
| **M0-02** | 配置加载与启动校验 | M0-01 | 覆盖单元测试：必填项校验、环境覆盖、非法值拦截、禁止 `clientFoundRows` | `DONE` |
| **M0-03** | 结构化日志组件（`log/slog`） | M0-01 | 支持 Text/JSON 格式切换；日志级别可配；零额外三方依赖 | `DONE` |
| **M0-04** | 统一 API 响应信封与错误码 | M0-01 | 成功/失败 Envelope 格式一致；未识别错误兜底为 `internal` | `DONE` |
| **M0-05** | 接入 `golang-migrate` 数据库迁移 | M0-06 | 本地数据库执行初始迁移；验证版本推进与 dirty 状态管理机制 | `DONE` |
| **M0-06** | 基础设施连接装配（MySQL/Redis/MQ） | M0-02 | 依赖不可用时快速失败并给出可读原因；连接池配置生效 | `DONE` |
| **M0-07** | HTTP 服务骨架与探针中间件 | M0-06 | 挂载 `RequestID` 与 `AccessLog` 中间件；暴露 `/healthz`、`/readyz` 与 Prometheus `/metrics` 端点；停用 Redis 时 readyz=503 而 healthz=200 | `DONE` |
| **M0-08** | API 服务入口与优雅停机 | M0-07 | 监听 SIGTERM 信号，在途请求处理完毕后按序释放资源，无 Panic | `DONE` |
| **M0-09** | Worker 异步服务入口 | M0-06 | 独立进程入口，能正常启动与优雅退出 | `DONE` |
| **M0-10** | 配置文件模板与环境变量示例 | M0-02 | `configs/config.yaml` 与 `.env.example` 完整齐备 | `DONE` |
| **M0-11** | 本地 Compose 编排文件 | — | 宿主机端口映射：MySQL 3307、Redis 6380、RabbitMQ 5673 | `DONE` |
| **M0-12** | Makefile 工程脚本 | M0-11 | 支持 `make up / down / migrate / run / test / vet` | `DONE` |
| **M0-13** | GitHub Actions CI 工作流 | M0-12 | PR/Push 自动执行 `vet` + `test` + `build`；校验历史迁移脚本不变性 | `DONE` |
| **M0-14** | 项目快速上手 README | — | 包含 10 分钟本地构建启动指引与架构简述 | `DONE` |
| **M0-15** | M0 端到端全流程验证 | M0-08~13 | 全新环境下 `make up` → `make migrate` → `make run` → 探针与 metrics 正常 → `make test` 全绿 | `DONE` |

> **边界约定**：M0 阶段不建业务数据表、不写业务 API Handler，数据库中仅初始化 `schema_migrations` 表。

---

## 五、后续里程碑任务概览

- **M1 账号与鉴权**：用户数据模型与迁移 · 注册（bcrypt） · 登录与双 Token 签发 · Refresh 轮转与重放防护 · 登出撤销 · 鉴权与软鉴权中间件（`SoftJWTAuth`） · Redis 滑动窗口限流中间件（`RateLimiter`） · 资料接口 · 对应测试。
- **M2 视频核心**：视频与统计表迁移 · 本地存储抽象与 HTTP `/media` 路由（支持 HTTP 206 Range 分段请求） · 上传与 MIME 校验 · 视频发布 · 详情与作者作品流 · 视频下架 · 对应测试。
- **M2.5 分片续传**：上传会话表与状态机 · 分片 Init/Upload/Status/Complete · 位图控制（MySQL 权威 + Redis 读加速）· 文件合并并发锁 · 针对性续传测试。
- **M3 全局 Feed**：DB 基线查询与单调 ID 游标 · Redis ZSet 时间线写入与原子裁剪（Lua）· Floor 计算与冷热拼接 · 下架过滤与游标推进 · 逻辑过期 + Singleflight 页缓存 · 降级策略 · 8 组边界测试。
- **M4 互动与 Outbox**：点赞/关注表与关系统计 · 点赞事务（`RowsAffected` 驱动）· 评论（基于客户端 `idempotency_key` 唯一键防重放）与关注 · Outbox 表与事务落库 · 租约派发调度器 · 消费端幂等（`consumed_events`）· 关注流推拉结合分发与多路归并 · 对应测试。
- **M5 通知与实时推送**：系统通知表 · Outbox 消费生成通知 · 未读计数与列表 · 标记已读 · SSE Hub 与短期 Ticket 鉴权 · 60s 重叠窗口断线补拉 · 对应测试。
