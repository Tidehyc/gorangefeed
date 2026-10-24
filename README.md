# GoRangeFeed

> 短视频 Feed 流后端系统，覆盖「发布 → 分发 → 消费 → 互动 → 通知」全链路闭环，采用垂直切片架构（Vertical Slice），单体 API + 独立 Worker。

---

## 一、架构简述

GoRangeFeed 采用垂直切片（Vertical Slice）按业务领域组织代码，杜绝空壳分层：

* **API 入口 (`cmd/api`)**：依赖装配、探针中间件、统一信封响应、路由分发与优雅停机；
* **Worker 入口 (`cmd/worker`)**：独立进程，负责 Outbox 事件派发与异步消费；
* **业务域 (`internal/`)**：
  * `account`：用户注册、登录、双 Token（Access + Refresh）轮转；
  * `video`：单文件与分片上传、流式发布（HTTP 206 Range）；
  * `interaction`：点赞（事务内行级锁驱动）、评论（客户端 `idempotency_key` 幂等）；
  * `relation`：关注、取关、粉丝列表；
  * `feed`：全局时间线（Redis ZSet + MySQL 冷热拼接）、关注流（推拉结合）；
  * `notification`：通知落库、未读计数、SSE 实时推送；
  * `outbox`：可靠事件投递与分布式租约调度；
  * `infra`：MySQL、Redis、RabbitMQ 连接池装配、配置加载与日志监控。

---

## 二、技术栈选型

| 基础设施 / 组件 | 选型 | 宿主机端口映射 |
|---|---|---|
| 语言框架 | Go 1.22+ / Gin | `:8080` (API) |
| 关系型数据库 | MySQL 8.0 / GORM | `:3307` (映射容器 3306) |
| 缓存与索引 | Redis 7.0 | `:6380` (映射容器 6379) |
| 消息队列 | RabbitMQ 3-management | `:5673` (AMQP) / `:15673` (管理后台) |
| 数据库迁移 | golang-migrate | `migrations/` |
| 日志与监控 | `log/slog` + Prometheus | `/metrics` 端点 |

---

## 三、10 分钟本地快速上手

### 1. 环境准备
确保本地已安装：
* [Go 1.22+](https://golang.org/dl/)
* [Docker & Docker Compose](https://www.docker.com/)
* `make` 工具

### 2. 启动基础依赖容器
```bash
# 一键启动 MySQL 8.0, Redis 7.0, RabbitMQ
make up
```

### 3. 执行数据库初始迁移
```bash
# 执行数据库迁移（初始化 schema_migrations）
make migrate
```

### 4. 启动服务
```bash
# 运行 API 服务
make run

# 或在另一终端运行 Worker 服务
make worker
```

### 5. 探针与指标验证
```bash
# 检查存活探针 (Liveness)
curl http://127.0.0.1:8080/healthz
# 响应: {"status":"ok"}

# 检查就绪探针 (Readiness，依赖 MySQL/Redis/RabbitMQ)
curl http://127.0.0.1:8080/readyz
# 响应: {"components":{"mysql":"ok","rabbitmq":"ok","redis":"ok"},"status":"ready"}

# 查看 Prometheus 指标
curl http://127.0.0.1:8080/metrics
```

### 6. 测试与代码校验
```bash
# 静态代码检查
make vet

# 运行全量单元与集成测试（带覆盖率）
make test

# 编译所有二进制产物至 bin/ 目录
make build
```

---

## 四、核心工程约束

1. **响应格式统一**：
   * 成功：`{"data": ..., "error": null}`
   * 失败：`{"data": null, "error": {"code": "...", "message": "..."}}`
2. **禁止 `clientFoundRows`**：
   * 点赞/互动链路依赖 MySQL `RowsAffected` 进行计数判定，配置加载时强校验严禁开启 `clientFoundRows=true`。
3. **迁移脚本不可变性**：
   * `migrations/` 历史已合并脚本严禁更改或删除，仅支持新增版本递增脚本。
