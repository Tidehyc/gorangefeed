# GoRangeFeed

> 短视频 Feed 流后端系统，覆盖「发布 → 分发 → 消费 → 互动 → 通知」全链路闭环，采用垂直切片架构（Vertical Slice），单体 API + 独立 Worker。

---

## 一、架构简述

GoRangeFeed 遵循**整洁架构（Clean Architecture）**分层设计，领域层（Domain）保持纯粹零外部框架依赖，通过依赖倒置驱动基础设施；端到端（E2E）黑盒测试与模块内白盒单测严格分流：

* **API 入口 (`cmd/api`)**：依赖注入装配、探针中间件、统一信封响应、路由组分发与平滑优雅停机；
* **Worker 入口 (`cmd/worker`)**：独立进程，负责 Outbox 事件派发与异步消费；
* **领域核心 (`internal/domain/`)**：纯业务实体、值对象与 Repository/TokenStore 抽象契约（覆盖 `account`、`video`、`interaction`、`relation`、`feed`、`notification`、`outbox`）；
* **应用服务 (`internal/service/`)**：业务用例编排、状态机控制与事务协调（如双 Token 轮转与重放防护）；
* **传输接口 (`internal/http/`)**：Gin 控制器、请求/响应 DTO、参数绑定校验与路由组挂载；
* **持久化基础设施 (`internal/infra/persistence/`)**：GORM MySQL 实体仓储与 Redis Lua 脚本会话存储实现；
* **基础设施 (`internal/infra/`)**：MySQL、Redis、RabbitMQ 连接池装配、JWT 管理、本地头像与媒体存储、配置加载与日志监控；
* **测试中心 (`test/`)**：
  * `test/e2e/`：独立运行的端到端黑盒生命周期测试；
  * `test/testutil/`：集成测试脚手架与 HTTP 辅助库；
  * 模块单元测试（`*_test.go`）保留在各层代码旁，便于白盒测试。

---

## 二、技术栈选型

| 基础设施 / 组件 | 选型 | 宿主机端口映射 |
|---|---|---|
| 语言框架 | Go 1.22+ / Gin | `:8080` (API) / `:9091` (Worker Metrics) |
| 关系型数据库 | MySQL 8.0 / GORM | `:3307` (映射容器 3306) |
| 缓存与索引 | Redis 7.0 | `:6380` (映射容器 6379) |
| 消息队列 | RabbitMQ 3-management | `:5673` (AMQP) / `:15673` (管理后台) |
| 监控中心 | Prometheus 3.1.0 | `:9090` (Prometheus Web UI) |
| 数据库迁移 | golang-migrate | `migrations/` |
| 日志与监控 | `log/slog` + Prometheus | API `:8080/metrics` / Worker `:9091/metrics` |

---

## 三、10 分钟本地快速上手

### 1. 环境准备
确保本地已安装：
* [Go 1.22+](https://golang.org/dl/)
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
