# GoRangeFeed 前端需求规格说明书（M14 · Web 客户端正式化）

> 本文档定义 GoRangeFeed Web 客户端的页面结构、交互行为、功能需求（FR）与非功能需求（NFR），
> 作为前端实现、联调与验收的唯一基准。
>
> 与 [`docs/REQUIREMENTS.md`](REQUIREMENTS.md) 的关系：该文档描述**后端能力**，
> 本文档描述**这些能力如何被用户看见和操作**，两者共用同一套编号与验收口径。
> 前端不新增 mock 服务、不伪造数据：页面上的每一个数字都必须来自真实接口返回。

---

## 一、项目定位与目标

### 1.1 客户端定位

面向移动端优先（mobile-first）的短视频 Web 客户端，覆盖「刷 → 赞 → 评 → 藏 → 关注 → 拍 → 管」全链路。
设计目标不是"能演示"，而是"能当产品用"：游客打开即刷，登录后可创作，刷新页面不丢状态，出错有明确出路。

### 1.2 与 M10 演示页的关系

现有 `web/index.html`（单文件 959 行，M10 交付）是**技术演示页**，已达成"能播、能赞、能看 SSE"的目标，
但存在三个不可延展的问题：无路由、无状态管理、UI 中混入 QoS 调试面板等演示元素。

本期（M14）以正式客户端**替换**它：

| 处置 | 内容 |
|---|---|
| 归档 | 旧页移至 `web/legacy/index.html`，保留一份可回看的演示快照，不再参与构建 |
| 保留能力 | 上下滑播放、双击点赞动效、SSE 通知、QoS 上报（**只上报，不再展示面板**） |
| 移除 | 调试抽屉、模拟数据按钮、内联测试脚本、`console.log` 级联输出、硬编码 token |
| 新增 | 路由、鉴权态、游标分页状态机、评论抽屉、发布流、个人中心、关系列表 |

### 1.3 边界与范围

- **包含**
  - 视频消费：推荐流、关注流、话题流、单视频深链；全屏竖向滑动；相邻预加载；曝光与播放行为上报。
  - 互动：点赞/取消、评论（一级 + 单层回复）、收藏（默认收藏夹）、分享（复制链接 / Web Share）。
  - 账号：注册、登录、静默刷新、登出、资料编辑、头像上传、修改密码、注销账户。
  - 创作：单文件上传与分片续传二选一（按文件大小自动切换）、封面、标题、描述、话题标签、发布。
  - 社交：关注/取关、关注列表、粉丝列表、他人主页、我的主页。
  - 通知：未读数、通知列表、SSE 实时到达。
  - 工程：`npm run build` 产物直接由 Go 服务同源托管；无 CORS 依赖。
- **排除**
  - 运营后台 UI（`/api/v1/admin/*` 已有接口，本期不做页面）。
  - 私信、直播、合拍、剪辑、滤镜、音乐库（后端无对应能力）。
  - 全文搜索（后端无搜索接口；话题页是唯一的内容聚合入口）。
  - HLS/ABR 多码率播放（等 M12 转码流水线落地）。
  - 原生 App、小程序（本期只做 Web）。
  - 推荐算法的策略调整（M11 后端工作，前端只消费 `scene=recommend`）。

### 1.4 验收标准

1. **一条命令起得来**：`make up && make migrate && make run` 后，浏览器打开 `http://127.0.0.1:8080/` 即为新客户端，无需另起 node 进程。
2. **全链路真实**：从注册到发布到点赞到评论，刷新页面后数据仍在数据库里；断网重连后状态可恢复。
3. **移动端可用**：375×667 视口下无横向滚动、无元素遮挡、底部安全区正确；桌面端 1440 宽不为"拉伸的手机页面"。
4. **异常有出口**：任一接口失败都有可见提示与重试入口，不出现白屏或无限骨架屏。
5. **可演示**：README 中给出明文测试账号，评审者按第八节脚本可在 10 分钟内走完全流程。

---

## 二、角色与核心场景

### 2.1 用户角色

| 角色 | 权限与能力 | 说明 |
|---|---|---|
| **游客** | 浏览推荐/热门/话题流、看评论、看他人主页 | 软鉴权接口，未登录即匿名；点"关注/点赞"时引导登录 |
| **登录用户** | 游客能力 + 点赞/评论/收藏/关注/通知/发布 | 一期不做角色分级，所有注册用户权限相同 |
| **创作者** | 即登录用户，额外使用发布、作品管理、数据查看 | 不单独设角色，进入 `/publish` 即创作态 |

> 游客点击"关注"必须先登录：`/feed/following` 是硬鉴权接口，游客访问会直接 401，前端不能靠"空列表"掩盖。

### 2.2 核心场景

1. **冷启动刷流（游客）**：打开首页 → 3 张骨架 → 首条视频静音自动播 → 上滑切换 → 第 3 条时预取下一页 → 滑到底显示"没有更多了"。
2. **互动闭环（登录用户）**：双击点赞（红心动画 + 计数 +1）→ 打开评论抽屉发一条评论 → 收藏 → 点作者头像进主页 → 关注 → 切到"关注"tab 看到该作者新作品。
3. **创作发布（创作者）**：点底部"+" → 选择文件（≤20MB 走单文件，更大走 5MB 分片续传，上限 200MB）→ 上传进度 → 填标题/描述/话题 → 发布成功 → 跳转个人主页首条即新作品。
4. **账号治理（登录用户）**：设置页改昵称 → 改密码（旧 token 立即失效）→ 注销账户（需输入密码 + 输入昵称确认）→ 回到登录页。
5. **异步触达（登录用户）**：他人点赞我的视频 → 顶部铃铛未读数 +1 → 点开通知列表 → 点击跳转对应视频。

---

## 三、信息架构与路由

### 3.1 路由表

| 路由 | 页面 | 核心接口 | 鉴权 | 优先级 |
|---|---|---|---|---|
| `/` | 首页 Feed（推荐 / 关注 双 tab） | `GET /feed/recommend`、`GET /feed/following` | 软 / 硬 | P0 |
| `/login` | 登录 / 注册（同页切换） | `POST /auth/login`、`POST /auth/register` | 公开 | P0 |
| `/publish` | 发布页 | `POST /videos/upload`、`POST /videos`、`POST /videos/chunk/*` | 登录 | P0 |
| `/me` | 我的主页（作品 + 资料） | `GET /users/me`、`GET /users/:id/videos` | 登录 | P0 |
| `/user/:id` | 他人主页 | `GET /users/:id`、`GET /users/:id/videos`、`GET /users/:id/relation` | 软 / 登录 | P0 |
| `/user/:id/followers` | 粉丝列表 | `GET /users/:id/followers` | 软 | P0 |
| `/user/:id/followings` | 关注列表 | `GET /users/:id/followings` | 软 | P0 |
| `/video/:id` | 单视频深链（分享落地页） | `GET /videos/:id`、`GET /videos/:id/comments` | 软 | P0 |
| `/tag/:tag` | 话题页 | `GET /tags/:tag/videos` | 软 | P1 |
| `/notifications` | 通知列表 | `GET /notifications`、`PATCH /notifications/:id/read` | 登录 | P1 |
| `/settings` | 账号设置 | `PUT /users/me`、`PUT /users/me/password`、`DELETE /users/me`、`POST /users/avatar` | 登录 | P0 |
| `/favorites` | 收藏与观看历史 | `GET /user/favorites`、`GET /user/history` | 登录 | P2 |
| `*` | 404（带"回首页"按钮） | — | 公开 | P0 |

### 3.2 导航结构

- **移动端**：底部 4 项 + 中央凸起发布键 —— `首页` / `发布(+)` / `消息` / `我的`；首页顶部叠加 `关注 | 推荐` 双 tab。
- **桌面端（≥1024px）**：左侧固定窄栏（图标 + 文案）承载同一组入口；Feed 区域居中，最大宽度按视频 9:16 计算，两侧留黑。
- **层级约束**：评论、分享、更多操作均为**浮层**，不改变路由；进入个人主页/设置/发布为**页面跳转**，浏览器返回键必须可用。

---

## 四、功能需求清单（FR）

> 优先级约定：
> - **P0**：本期必须交付，缺失即视为功能不完整。
> - **P1**：应交付项，时间紧张时可延后一版。
> - **P2**：锦上添花，不阻塞验收。

### 4.1 应用外壳与全局状态（Shell - M14.1）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-SHELL-01** | 路由与懒加载 | P0 | 每个一级页面独立 chunk；首屏 JS gzip 后 ≤ 180KB；切换页面无白屏闪烁 | — |
| **FR-SHELL-02** | 鉴权态全局管理 | P0 | 刷新页面后仍保持登录；`/me`、`/publish`、`/settings` 未登录访问时重定向 `/login?redirect=<原路径>`，登录后回跳 | `GET /users/me` |
| **FR-SHELL-03** | 全局反馈组件 | P0 | 成功/失败均有 Toast；同一错误 3 秒内不重复堆叠；错误文案取后端 `error.message`，不再二次翻译 | 全部 |
| **FR-SHELL-04** | 错误边界 | P0 | 任一页面渲染异常时展示兜底页 + "返回首页"，不出现整页白屏 | — |
| **FR-SHELL-05** | 移动端安全区 | P0 | iPhone 刘海/底部横条机型上，底部导航与操作栏不被裁切（`env(safe-area-inset-*)`） | — |
| **FR-SHELL-06** | 断网提示 | P1 | `navigator.onLine=false` 时顶部显示离线条；恢复后自动重试当前页请求 | — |

### 4.2 视频流（Feed - M14.2）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-FEED-01** | 全屏竖向滑动 | P0 | 一屏一条，CSS scroll-snap 吸附；滑动跟手、无回弹错位；桌面滚轮一格一条 | `GET /feed/recommend` |
| **FR-FEED-02** | 自动播放与静音策略 | P0 | 首条进入视口 60% 即自动播放（默认静音）；右上角"开启声音"按钮，点击后全局生效并持久化；浏览器拦截自动播放时降级为点击播放 | — |
| **FR-FEED-03** | 切换暂停与恢复 | P0 | 切走的视频立即 `pause()`；切回时从上次位置 `resume()`；同时最多 3 个 `<video>` 实例存在 | — |
| **FR-FEED-04** | 单击暂停 / 双击点赞 | P0 | 单击延迟 250ms 判定为播放/暂停；300ms 内两次点击触发点赞 + 红心动画；双击**只点赞不取消**；动画在点击坐标处播放，400ms 内结束 | `POST /videos/:id/like` |
| **FR-FEED-05** | 键盘与手势 | P1 | 桌面：`↑/↓` 切换、`space` 播放暂停、`M` 静音、`←/→` 快退/快进 5s；移动端：上下滑即切换（不额外绑定 touch 事件） | — |
| **FR-FEED-06** | 相邻预加载 | P1 | 当前条 `preload="auto"`，下一条 `preload="metadata"`，其余不加载；弱网（`navigator.connection.saveData`）下全部降为 metadata | — |
| **FR-FEED-07** | 首屏骨架 | P0 | 首屏 3 张骨架卡，含头像/文字/操作栏占位，带 shimmer；命中数据后无布局跳动（CLS < 0.1） | — |
| **FR-FEED-08** | 游标分页与无限加载 | P0 | 当前索引 ≥ `items.length - 3` 时预取下一页；同一 `id` 不重复插入；`has_more=false` 后不再请求并显示"已经到底了"；请求中不并发发起第二次 | 各 Feed 接口 |
| **FR-FEED-09** | 分页失败可恢复 | P0 | 翻页失败保留已加载内容，底部出现"加载失败，点击重试"；重试复用同一 cursor | 同 FR-FEED-08 |
| **FR-FEED-10** | 卡片信息区 | P0 | 左下显示 `@昵称`、标题（≤2 行）、描述（≤3 行，超出省略）；作者头像可点击进主页；`is_followed=false` 时头像下方显示"+"关注按钮 | — |
| **FR-FEED-11** | 右侧互动栏 | P0 | 自上而下：作者头像、点赞、评论、收藏、分享；点赞/收藏为实心态切换（红/黄）；计数用 `1.2w` 格式（≥10000 时）；点赞按钮无计数时显示"点赞" | 见 4.3 |
| **FR-FEED-12** | 播放进度条 | P1 | 底部 2px 进度条随播放推进；可点击跳转；拖动时显示当前时间/总时长；视频结束显示重播按钮 | — |
| **FR-FEED-13** | 播放失败占位 | P0 | `video.onerror` 时展示封面 + "播放失败，点击重试"；连续 3 条失败顶部提示"网络异常，请检查连接" | — |
| **FR-FEED-14** | 曝光与播放上报 | P0 | 进入视口播放 1s 后上报 `play`（含 `event_type`/`watch_duration_ms`）；离开或播放 ≥90% 上报 `complete`；同一视频同一会话内 `play` 只上报一次 | `POST /videos/:id/view` |
| **FR-FEED-15** | 双 tab 独立状态 | P0 | "关注"与"推荐"各自维护列表、游标、滚动位置；切换 tab 不重置另一侧已加载内容；"关注"tab 未登录时展示登录引导而非空列表 | `GET /feed/following` |
| **FR-FEED-16** | 空流兜底 | P1 | 推荐流返回空（如首装无数据）时自动回退 `GET /feed/global` 并提示"正在为你准备内容"；关注流为空提示"还没有关注的人"，给"去发现"按钮 | `GET /feed/global` |
| **FR-FEED-17** | 话题标签 | P0 | 描述下方渲染 `tags` 芯片（`#话题`），点击进入 `/tag/:tag`；后端未返回 `tags` 时从描述中解析 `#xxx` 兜底，同一 tag 不重复渲染 | `GET /tags/:tag/videos` |

### 4.3 互动（Interaction - M14.3）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-INT-01** | 点赞（乐观更新） | P0 | 点击立即变红 + 计数 +1，请求失败回滚并 Toast；300ms 内连点只发一次请求；未登录点击跳登录并保留回跳地址 | `POST /videos/:id/like` |
| **FR-INT-02** | 取消点赞 | P0 | 再次点击取消，计数 -1 且不小于 0；与服务端 `like_count` 最终一致（重新进页面后对齐） | `DELETE /videos/:id/like` |
| **FR-INT-03** | 收藏 | P0 | 默认收藏到默认收藏夹（不传 `folder_id`）；实心态切换；失败回滚；未登录跳登录 | `POST/DELETE /videos/:id/favorite` |
| **FR-INT-04** | 分享 | P0 | 移动端优先 `navigator.share`；不支持时复制 `/video/:id` 链接并提示"链接已复制"；**不展示分享计数**（后端无此数据，见 6.6） | — |
| **FR-INT-05** | 分享落地页 | P0 | 打开 `/video/:id` 直接定位到该视频并自动播放；视频已下架时展示"内容不存在或已下架" | `GET /videos/:id` |
| **FR-INT-06** | 关注/取关 | P0 | 作者头像下"+"点击即关注，按钮变为"已关注"；关注流 tab 需手动刷新才出现新内容（不做实时插入）；取关有二次确认 | `POST/DELETE /users/:id/follow` |
| **FR-INT-07** | 限流提示 | P1 | 收到 `rate_limited` 时 Toast "操作太频繁，请稍后再试"，按钮禁用 5s；不弹出原始错误信息堆栈 | 全部写接口 |

### 4.4 评论（Comment - M14.4）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-CMT-01** | 评论抽屉 | P0 | 移动端从底部升起占 65% 高度，可下拉关闭；桌面端为右侧 400px 面板；打开时不中断视频播放 | — |
| **FR-CMT-02** | 评论列表 | P0 | 展示头像、昵称、内容、时间（相对时间）、点赞数；游标分页（每页 10 条），滚到底自动加载；`sort=latest` 默认 | `GET /videos/:id/comments` |
| **FR-CMT-03** | 热评排序 | P2 | 支持切换"最热"，`sort=hot` 游标为 `likeCount_id` 复合串，前端原样透传 | 同上 |
| **FR-CMT-04** | 发表评论 | P0 | 输入框吸底，发送后插入列表顶部并计数 +1；空内容不可发送；每次请求生成新的 `Idempotency-Key`（UUID），重试复用同一个 key | `POST /videos/:id/comments` |
| **FR-CMT-05** | 回复 | P1 | 点击"回复"进入"回复 @昵称"态，提交时带 `parent_id`；回复展示在父评论下方并带"回复 @xxx"前缀；**仅单层**，回复的回复仍挂到根评论 | 同上 |
| **FR-CMT-06** | 删除自己的评论 | P1 | 自己的评论显示删除入口，二次确认后删除；删除后计数 -1 | `DELETE /comments/:id` |
| **FR-CMT-07** | 评论点赞 | P2 | 支持点赞/取消，计数乐观更新；未登录跳登录 | `POST/DELETE /comments/:id/like` |
| **FR-CMT-08** | 空态与失败 | P0 | 无评论显示"还没有评论，来抢沙发"；加载失败显示重试按钮；发送失败保留草稿并提示 | — |

> 后端评论列表不按 `parent_id` 过滤，回复与父评论同页返回。前端在同一页内按 `parent_id` 聚合；
> 跨页的孤立回复直接平铺展示，不伪造"展开 N 条回复"。

### 4.5 账号与鉴权（Auth - M14.5）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-AUTH-01** | 登录 | P0 | 用户名 + 密码；失败提示"用户名或密码错误"（不区分具体原因）；成功后写入 token 并回跳 `redirect` | `POST /auth/login` |
| **FR-AUTH-02** | 注册 | P0 | 用户名 3~32 位字母数字下划线、密码 8~32 位含字母与数字；前端预校验 + 后端错误原样展示（`conflict` 提示"用户名已被占用"）；注册成功后自动登录 | `POST /auth/register` |
| **FR-AUTH-03** | 凭证存储与刷新 | P0 | Access Token 仅存内存；Refresh Token 存 `localStorage`；启动时静默刷新；401 时单飞刷新一次并重放原请求，刷新失败则清理并跳登录 | `POST /auth/refresh` |
| **FR-AUTH-04** | 登出 | P0 | 调用接口注销会话后清空本地凭证并跳登录页；登出后浏览器回退不能恢复已登录界面 | `POST /auth/logout` |
| **FR-AUTH-05** | 密码变更后强制重登 | P0 | 修改密码成功后清除本地凭证并跳登录（后端已使用户旧 token 失效，前端不得继续用旧 token 重试） | `PUT /users/me/password` |
| **FR-AUTH-06** | 演示账号可见 | P1 | `/login` 页脚以纯文本展示 2 个测试账号与密码，点击可一键填入；生产构建通过 `VITE_SHOW_DEMO_ACCOUNTS` 控制 | — |

### 4.6 个人中心与社交关系（Profile - M14.6）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-PROF-01** | 我的主页 | P0 | 头像、昵称、简介、统计（关注/粉丝/作品/获赞）；下方为作品宫格（3 列），点击进入 `/video/:id` | `GET /users/me`、`GET /users/:id/videos` |
| **FR-PROF-02** | 统计数字真实 | P0 | 四个统计值来自后端 `stats`，为零时显示 0 而非隐藏；**前置条件：后端需补齐统计聚合**（见 6.6-G1） | 同上 |
| **FR-PROF-03** | 他人主页 | P0 | 展示昵称/头像/简介/统计 + 作品宫格；顶部右侧为"关注/已关注"按钮（互粉显示"互相关注"）；游客点击跳登录 | `GET /users/:id`、`GET /users/:id/relation` |
| **FR-PROF-04** | 资料编辑 | P0 | 昵称 ≤64 字、简介 ≤255 字，实时字数提示；保存后顶部资料立即更新 | `PUT /users/me` |
| **FR-PROF-05** | 头像上传 | P1 | 支持 jpg/png/webp ≤10MB；本地预览 + 上传进度；成功后全局头像同步更新 | `POST /users/avatar` |
| **FR-PROF-06** | 关注/粉丝列表 | P0 | 两个列表均可进入，展示头像、昵称、`@username`、简介；游标分页；列表项右侧有"关注/已关注"按钮（自己不出现在列表中） | `GET /users/:id/followings`、`GET /users/:id/followers` |
| **FR-PROF-07** | 列表空态 | P0 | 无关注显示"还没有关注任何人"；无粉丝显示"还没有粉丝"；列表加载失败可重试 | 同上 |
| **FR-PROF-08** | 作品管理 | P1 | 我的主页作品支持"删除"（后端语义为下架），二次确认；删除后从宫格移除 | `DELETE /videos/:id` |

### 4.7 发布（Publish - M14.7）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-PUB-01** | 选择文件 | P0 | 仅接受 `video/mp4`，≤200MB；超限/格式错误立即提示，不发请求；选择后展示文件名、大小、时长（读 metadata） | — |
| **FR-PUB-02** | 单文件上传 | P0 | ≤20MB 走单文件接口，显示百分比进度；成功后回填 `play_url` | `POST /videos/upload`（字段 `file`） |
| **FR-PUB-03** | 分片续传 | P1 | >20MB 走分片：5MB/片，先算整文件 SHA-256（64 位小写十六进制）再 `init`；单片失败重试 3 次；刷新页面后凭 `upload_id` 可续传 | `POST /videos/chunk/init`、`/upload`、`/complete` |
| **FR-PUB-04** | 封面 | P1 | 可从视频首帧截取（`canvas`）或上传图片（jpg/png/webp ≤10MB）；不传封面时后端允许，前端以渐变占位 | `POST /videos/cover` |
| **FR-PUB-05** | 元信息 | P0 | 标题必填 ≤128 字；描述 ≤1000 字；话题最多 5 个、每个 ≤32 字，输入框按 `#`/空格/逗号分词并去重，超限即时提示（不静默截断） | `POST /videos` |
| **FR-PUB-06** | 发布幂等 | P0 | 每次进入发布页生成一个 `idempotency_key`（UUID），重复点击只发一次请求；成功后重置 key 并跳转 `/me` | 同上 |
| **FR-PUB-07** | 发布成功反馈 | P0 | 成功后跳转我的主页，新作品位于首位；默认策略为先发后审，作品立即可见；若发布后作品未出现在列表（命中机审高风险被隐藏），提示"作品可能未通过审核，请检查标题与描述"而不静默消失 | — |
| **FR-PUB-08** | 离开保护 | P1 | 上传中离开页面弹出确认；已上传未发布的文件在 24h 内保留草稿（`localStorage` 记录 `play_url` 与表单） | — |

### 4.8 通知（Notification - M14.8）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-NOT-01** | 未读红点 | P1 | 顶部铃铛展示未读数（>99 显示 99+）；进入通知页后归零 | `GET /notifications/unread-count` |
| **FR-NOT-02** | 通知列表 | P1 | 按时间倒序，展示触发者头像、`title`、`content` 摘要、相对时间；按 `target_type` + `target_id` 决定跳转（video→`/video/:id`、user→`/user/:id`）；游标分页（默认 20 条） | `GET /notifications` |
| **FR-NOT-03** | 已读处理 | P1 | 点击单条标为已读并跳转目标；提供"全部已读" | `PATCH /notifications/:id/read`、`POST /notifications/read-all` |
| **FR-NOT-04** | SSE 实时到达 | P2 | 登录后取 ticket 建立 `/notifications/stream?ticket=`；新通知 1s 内到达并 Toast；断线指数退避重连（1s→30s 封顶） | `POST /notifications/sse/ticket` |
| **FR-NOT-05** | 游客不建连 | P1 | 未登录不发起 SSE；登出时主动关闭连接，避免服务端连接泄漏 | — |

### 4.9 话题（Topic - M14.9）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-TOPIC-01** | 话题页 | P1 | 展示 `#话题名`、作品数（按当前页推断，不承诺精确总数）、宫格或列表；点击作品进 `/video/:id` | `GET /tags/:tag/videos` |
| **FR-TOPIC-02** | 话题归一 | P1 | 路由参数需 `decodeURIComponent` 并去 `#`；大小写与后端一致（后端按小写归一），前端跳转时统一小写 | 同上 |
| **FR-TOPIC-03** | 空话题 | P1 | 无作品显示"该话题下还没有内容"，给"回首页"按钮 | — |

### 4.10 收藏与历史（Asset - M14.10）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-ASSET-01** | 默认收藏夹 | P2 | 自动取/建默认收藏夹；列表展示封面、标题、时长 | `GET /user/favorites`、`GET /user/favorites/:id/items` |
| **FR-ASSET-02** | 观看历史 | P2 | 按时间倒序，支持单条删除与清空；点击续播 | `GET /user/history`、`DELETE /user/history` |
| **FR-ASSET-03** | 断点续播 | P2 | 进入视频时读取 `GET /videos/:id/history`，从上次位置继续（误差 ≤1s） | `GET/POST /videos/:id/history` |

### 4.11 通用状态与可访问性（UX - M14.11）

| 编号 | 功能需求 | 优先级 | 验收标准 | 接口 |
|---|---|---|---|---|
| **FR-UX-01** | 加载态规范 | P0 | 首屏用骨架；局部刷新用内联 spinner；按钮提交中禁用并显示 loading，防重复提交 | — |
| **FR-UX-02** | 空态规范 | P0 | 每类空态都有文案 + 主行动按钮（去发布/去发现/回首页），不使用纯图标 | — |
| **FR-UX-03** | 错误码映射 | P0 | `unauthorized`→跳登录；`forbidden`→"没有权限"；`not_found`→空态页；`rate_limited`→频控提示；`internal`→"服务异常，请稍后重试" | 全部 |
| **FR-UX-04** | 键盘可达 | P1 | 所有交互元素可 Tab 聚焦，有可见焦点环；视频控件有 `aria-label`；`prefers-reduced-motion` 时关闭红心粒子动画 | — |
| **FR-UX-05** | 图片降级 | P1 | 头像/封面加载失败显示首字母或稳定渐变色块，不显示浏览器破图图标 | — |
| **FR-UX-06** | 无调试残留 | P0 | 生产构建不含演示按钮、mock 开关、QoS 面板、`console.log`（保留 `console.error`） | — |

---

## 五、非功能需求（NFR）

### 5.1 性能（PERF）

- **NFR-FEPERF-01**：本地环境（`127.0.0.1`），首屏可交互 < 2.0s，LCP < 2.5s；首条视频首帧 < 1.5s（种子视频 ≤5MB）。
- **NFR-FEPERF-02**：首屏 JS gzip ≤ 180KB，CSS ≤ 40KB；字体使用系统栈，不加载远程字体。
- **NFR-FEPERF-03**：滑动切换响应 < 100ms；不得因预加载导致当前视频卡顿（同时解码实例 ≤ 2）。
- **NFR-FEPERF-04**：列表 ≥ 100 条时滑动仍保持 55fps 以上（Chrome DevTools Performance 采样）。

### 5.2 兼容与响应式（COMPAT）

- **NFR-COMPAT-01**：支持最近两个大版本的 Chrome / Edge / Safari / Firefox；iOS Safari 15+ 必须可播（含 `playsinline`）。
- **NFR-COMPAT-02**：断点：`<768px` 移动布局、`768–1023px` 平板（Feed 居中 + 底部导航）、`≥1024px` 桌面（左侧栏 + 居中 Feed）。
- **NFR-COMPAT-03**：375×667 与 1440×900 两个尺寸下均无横向滚动条、无文字截断错位。

### 5.3 健壮性（REL）

- **NFR-FEREL-01**：任何接口失败都有用户可见反馈，不吞错、不无限重试；自动重试仅限幂等 GET，最多 2 次指数退避。
- **NFR-FEREL-02**：Token 刷新采用单飞（single-flight），并发 401 只触发一次刷新，其余请求排队等待。
- **NFR-FEREL-03**：页面刷新/浏览器前进后退后，Feed 位置、tab、登录态可恢复（`sessionStorage` 记录场景与索引）。
- **NFR-FEREL-04**：上传失败可续传，不要求用户重新选择文件（分片模式）。

### 5.4 安全（SEC）

- **NFR-FESEC-01**：不引入 `dangerouslySetInnerHTML`；评论、昵称、简介一律按文本渲染，防 XSS。
- **NFR-FESEC-02**：Access Token 不落 `localStorage`、不写 URL；不打印到控制台。
- **NFR-FESEC-03**：媒体一律走同源 `/media/*`，不拼接后端内网地址；`play_url` 只接受 `/media/` 开头的相对路径（后端返回即如此）。
- **NFR-FESEC-04**：注销账户需输入密码 + 手动输入昵称确认；操作不可撤销，按钮为危险色。
- **NFR-FESEC-05**：不在前端存储任何后端密钥；所有权限判断以接口返回为准，前端隐藏按钮不作为安全边界。

### 5.5 可维护性（MAINT）

- **NFR-FEMAINT-01**：TypeScript `strict: true`，禁止 `any`（确需时用 `unknown` + 收窄）；接口响应有显式类型定义。
- **NFR-FEMAINT-02**：目录按「页面 / 组件 / 领域 hooks / API / 类型」分层，单个组件 ≤ 250 行；超过则拆分。
- **NFR-FEMAINT-03**：所有接口调用集中在 `src/api/`，组件内不直接 `fetch`；请求封装统一处理信封解包、错误码、鉴权刷新。
- **NFR-FEMAINT-04**：不引入 UI 组件库（避免体积与样式冲突），基础组件自建：Button / Modal / Drawer / Toast / Skeleton / Avatar。

### 5.6 可观测性（OBS）

- **NFR-FEOBS-01**：播放 QoS 静默上报（首帧耗时、卡顿次数与时长、播放时长、是否完播）到 `POST /playback/events`，**不在 UI 展示**。
- **NFR-FEOBS-02**：前端错误上报到 `console.error` 并带 `request_id`（从响应头读取），便于与后端日志对齐。

---

## 六、接口契约要点（前端必须遵守）

### 6.1 统一信封与错误码

```json
{ "data": {...}, "error": null }
{ "data": null, "error": { "code": "invalid_param", "message": "Invalid cursor" } }
```

错误码全集：`invalid_param` / `unauthorized` / `forbidden` / `not_found` / `conflict` / `rate_limited` / `internal`。
HTTP 状态码不可作为唯一判据（业务错误与状态码并非一一对应），**以 `error.code` 为准**。

### 6.2 分页游标（关键差异）

游标类型在不同场景**不一致**，前端一律按**不透明字符串**处理，不做数值运算：

| 场景 | 游标类型 | 首页游标 |
|---|---|---|
| `feed/global`、`feed/following`、`tags/:tag/videos` | 数值 ID（JSON number） | `0` |
| `feed/hot`、`feed/recommend`、`feed/like-rank` | Base64 字符串 | 空串 |
| `comments`（`sort=latest`） | 时间戳字符串 | 空串 |
| `comments`（`sort=hot`） | `likeCount_id` 复合串 | 空串 |

约束：`has_more=false` 时不得再发请求；同一游标重复请求结果必须一致；游标不得被前端解析或构造。

### 6.3 鉴权与刷新

- 请求头 `Authorization: Bearer <access_token>`；软鉴权接口不带 token 即游客视角。
- 401 处理：单飞刷新 → 重放一次 → 仍失败则清凭证跳 `/login`。
- `PUT /users/me/password`、`DELETE /users/me` 成功后，服务端已使用户全部旧 token 失效（Redis `auth:invalid_before:{uid}`），
  前端必须清本地凭证，**不要**用旧 token 重试。

### 6.4 上传

| 场景 | 接口 | 要点 |
|---|---|---|
| 单文件视频 | `POST /videos/upload`，multipart 字段名 `file` | ≤200MB；MP4 魔数校验；返回 `{play_url, size}` |
| 封面 | `POST /videos/cover`，字段名 `file` | jpg/png/webp ≤10MB；返回 `{cover_url}` |
| 头像 | `POST /users/avatar`，字段名 `file` | 同上 |
| 分片 | `POST /videos/chunk/init` → `upload` → `complete` | 默认 5MB/片；`file_hash`/`chunk_hash` 为 64 位小写十六进制 SHA-256；`upload` 使用 form 字段 `upload_id`、`chunk_index`、`chunk_hash`、`file` |

### 6.5 响应结构差异（需要适配层）

Feed 与详情返回的是**两种不同结构**，前端需要 `normalizeVideo()` 统一成同一视图模型：

| 来源 | 结构 |
|---|---|
| `GET /feed/*`、`GET /tags/:tag/videos` | 扁平：`{id, author_id, title, description, cover_url, play_url, duration_ms, published_at, author{...}, stats{like_count, comment_count, view_count}, is_liked, is_followed, tags[]}` |
| `GET /videos/:id`、`GET /users/:id/videos`、`GET /users/me/likes` | 嵌套：`{video{...}, stats{...含 favorite_count}, author{...}, is_liked, is_favorited}` |

差异点：① 详情多 `favorite_count`/`is_favorited`，Feed 少；② 详情无 `is_followed` 与 `tags`；③ 详情时间字段在 `video` 内。
统一层必须显式处理"缺失字段用默认值"，不得把 `undefined` 直接渲染成 `NaN`。

### 6.6 已知后端缺口与前端应对

> 这几项必须在**前端开发前**确认处置方式，否则页面会出现"数字永远是 0"或"按钮点了没反应"。

| 编号 | 缺口 | 影响 | 前端应对（若后端不补） | 建议 |
|---|---|---|---|---|
| **G1** | `GET /users/me`、`GET /users/:id` 的 `stats` 恒为 0（`ToUserProfileResponse` 里写死 `UserStats{}`） | 个人主页/他人主页的关注数、粉丝数、作品数、获赞数全部显示 0 | 无法在前端补；只能隐藏统计区 | **P0 补齐**：`account` service 注入 relation/video/asset 计数器，聚合查询后填充 |
| **G2** | Feed 的 `stats` 只有 `like_count`/`comment_count`/`view_count`，缺 `favorite_count`、缺 `is_favorited` | 互动栏收藏数无法展示；进页面时收藏态需额外请求 | 收藏数不显示；收藏态通过 `GET /videos/:id` 单条补查（N+1，不可接受） | **P0 补齐**：`FeedStat` 增加 `favorite_count`，`FeedItem` 增加 `is_favorited`（按 viewer 批量查） |
| **G3** | 无分享接口、无 `share_count` | 分享只能复制链接，不能计数 | 分享按钮不显示计数（已写入 FR-INT-04） | P1 可选：`POST /videos/:id/share` + `share_count` |
| **G4** | `GET /users/:id` 不返回 `username`，也不返回关注态 | 他人主页无法展示 `@username`；游客无法得知是否已关注 | 昵称代替；关注态调 `GET /users/:id/relation`（需登录，游客跳过） | P1 可选：公开资料增加 `username` 与 `is_following` |
| **G5** | 评论列表无 `parent_id` 过滤、无 `reply_count` | 无法做"展开 N 条回复" | 同页内聚合（已写入 4.4 说明） | P2 可选 |
| **G6** | 无搜索、无"不感兴趣"反馈接口 | 无法做搜索页与负反馈 | 不做（已列入排除项） | P2 可选 |
| **G7** | `GET /users/:id/videos` 只返回 `status=published`，作者看不到自己被机审隐藏/下架的作品，也无审核状态接口 | 发布后若被机审隐藏，作品在"我的主页"凭空消失，用户无法自查 | 用"新作品未出现在列表"的提示兜底（FR-PUB-07） | P1 可选：作者视角作品列表返回 `hidden` 并附 `moderation_status` |

> 处置约定：G1、G2 作为 M14-01 的后端前置任务，先补后端再写页面对应区域；
> G3~G7 明确降级，不阻塞验收，但需在 README 的"已知限制"中如实说明。

---

## 七、技术选型与工程结构

### 7.1 技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| 构建 | Vite 5 | 冷启动快，产物小，`base: '/'` 直接对接 Go 静态托管 |
| 框架 | React 18 + TypeScript（strict） | 团队/生态熟悉度与类型安全 |
| 路由 | React Router 6 | 支持嵌套路由与 `redirect` 回跳 |
| 样式 | Tailwind CSS 3（内置 `line-clamp`，无需额外插件） | 原子类便于移动端快速对齐，无需引入组件库 |
| 状态 | Zustand（auth / player / feed 三个 store） | 轻量，避免 Redux 样板代码 |
| 数据 | 自研 `useCursorFeed` hook + `apiFetch` 封装 | 游标分页逻辑统一，避免引入重型 query 库 |
| 图标 | 内联 SVG 组件 | 不引入整套图标库，控制体积 |

### 7.2 目录结构

```
web/
  index.html                 # Vite 入口（替换旧演示页）
  legacy/index.html          # M10 演示页归档，不参与构建
  package.json
  vite.config.ts             # dev 代理 /api、/media → 127.0.0.1:8080
  tailwind.config.js
  src/
    main.tsx
    App.tsx                  # 路由表 + 全局 Provider
    api/                     # client.ts（信封/401/单飞刷新）、auth.ts、feed.ts、video.ts、interaction.ts、relation.ts、asset.ts、notification.ts
    types/                   # 与后端 DTO 一一对应的类型
    stores/                  # auth.ts、player.ts、feed.ts
    hooks/                   # useCursorFeed.ts、useAuthGuard.ts、useVideoPlayback.ts、useIntersection.ts
    components/              # VideoCard、ActionBar、CommentDrawer、Skeleton、Toast、Avatar、EmptyState、ErrorState
    pages/                   # Feed、Login、Publish、Me、UserProfile、FollowList、VideoDetail、Tag、Notifications、Settings、Favorites、NotFound
    styles/
```

### 7.3 联调与部署

- **开发态**：`npm run dev`（5173）→ Vite proxy 把 `/api`、`/media` 转发到 `127.0.0.1:8080`。
  后端**没有 CORS 中间件**，禁止前端直连 8080 跨域请求。
- **生产态**：`npm run build` 输出到 `web/dist`，再由 Go 侧挂载。当前 `MountWebRoutes(r, "web")` 只挂了 `/` 与 `/web` 静态目录，
  需要改为指向 `web/dist`（或构建产物直接落到 `web/` 根，二选一，以改动小者为准），并补 **SPA history fallback**：
  非 `/api`、`/media`、`/healthz`、`/readyz`、`/metrics` 的未命中请求返回 `index.html`，
  否则刷新 `/user/12` 会 404。回退实现用 `NoRoute`，不得用通配 `r.GET("/*any")` 覆盖 API 路由。
- 媒体路径统一 `/media/...`，与前端同源，无跨域与额外签名。

### 7.4 种子数据与演示账号

`make seed` 需产出（脚本 `scripts/seed.sh` 或 Go 子命令，落地时二选一）：

| 内容 | 要求 |
|---|---|
| 账号 | 6 个：`alice`/`bob`/`carol`/`dave`/`erin`/`frank`，密码统一 `Passw0rd!`；`alice` 为创作主力（≥6 条作品） |
| 视频 | 12~18 条，每条约 5~15s、≤5MB，全部落在 `uploads/videos/`；封面可为空（前端用稳定渐变色占位） |
| 关系 | alice 关注 bob/carol；bob/carol 回关 alice（构造 `is_friend=true` 场景）；dave 无关注（关注流空态） |
| 互动 | 每条视频 0~20 个赞、0~5 条评论（含 1 条回复），保证热榜/推荐非空 |
| 标签 | 每条 2~4 个，覆盖 `旅行`/`美食`/`城市`/`宠物` 四类，确保 `/tag/旅行` 有内容 |
| 凭据 | 明文写入 README 与 `/login` 页脚；不使用 `admin` 账号做演示 |

素材来源必须可商用（CC0 / Pexels 等），并在 README 注明来源与许可以免版权争议。

---

## 八、验收脚本（手工，10 分钟）

| 步骤 | 操作 | 预期 |
|---|---|---|
| 1 | 未登录打开 `/` | 3 张骨架 → 视频静音自动播；顶部"开启声音"；右侧计数与数据库一致 |
| 2 | 上滑 5 次 | 每次切换流畅，上一条立即暂停；滑到底出现"已经到底了"，无重复视频 |
| 3 | 双击视频 | 点击处出现红心动画，点赞数 +1，刷新后仍为已赞 |
| 4 | 点"关注"tab（未登录） | 跳转 `/login?redirect=/`，不是空列表 |
| 5 | 用 `alice` 登录 | 回到首页且 tab 保持；右上角出现头像与铃铛 |
| 6 | 打开评论抽屉，发一条评论 | 列表顶部出现该评论，计数 +1；再发同样内容不会被去重误伤（新 key） |
| 7 | 点作者头像 → 关注 | 按钮变"已关注"；进入 `/user/:id/followers` 能看到 alice |
| 8 | 进 `/settings` 改昵称 | 保存后底部导航与 Feed 卡片昵称同步变化 |
| 9 | 改密码（旧→新） | 成功后跳登录；用旧密码登录失败，新密码成功 |
| 10 | `/publish` 上传 ≤20MB 视频，填标题与 3 个话题 | 进度 → 成功 → 跳 `/me`，首条为新作品，卡片显示 3 个话题芯片 |
| 11 | 点话题芯片 | 进入 `/tag/xxx`，能看到该视频 |
| 12 | 注销账户（输入密码 + 昵称） | 跳登录；该用户作品从 Feed 消失；其评论作者显示"已注销用户" |
| 13 | 桌面端刷新 `/user/1` | 正常渲染该页面（验证 history fallback），不是 404 |
| 14 | 断网后上滑 | 出现"加载失败，点击重试"，恢复网络后重试成功 |

---

## 九、与抖音/TikTok 的差距与优化机会

### 9.1 能力对照

| 维度 | 抖音 / TikTok | 本项目（M14） | 差距性质 |
|---|---|---|---|
| 分发 | 多目标模型（完播/互动/负反馈）实时排序 | 规则召回（热 50% + 新 30% + 探索 20%）+ 曝光去重 + 同作者打散 | 算法深度（M11 逐步补） |
| 播放器 | 原生硬解、双缓冲、首帧 <100ms | HTML5 `<video>` + 相邻预取 + 封面占位 | Web 平台限制 |
| 画质 | 多码率 ABR、按网速切换 | 原文件直出，无转码 | 缺 M12 转码流水线 |
| 互动 | 赞/评/藏/分享（含计数）、合拍、弹幕 | 赞/评/藏/分享（**无分享计数**） | 后端缺字段 |
| 评论 | 无限层级楼中楼、作者赞过、置顶 | 一级 + 单层回复，同页聚合 | 数据模型限制 |
| 创作 | 拍摄、剪辑、滤镜、音乐、字幕 | 只能上传成片 | 产品形态不同 |
| 推荐反馈 | 长按"不感兴趣"、减少此类内容 | 无 | 缺接口 |
| 通知 | 全类型 + 聚合（"xxx 等 3 人赞了你"） | 单条通知 + 未读数 + SSE | 缺聚合逻辑 |
| 弱网 | 预加载策略、清晰度自动降级 | 骨架 + 失败重试 + 数据节省模式 | 可继续优化 |
| 数据 | 创作者中心（完播率、粉丝画像） | 只有作品列表与总获赞 | 缺分析接口 |

### 9.2 可落地优化清单（按投入产出排序）

1. **首帧体验**（前端，1 天）：封面图优先渲染 + 当前条 `preload="auto"` + 下一条 metadata，首帧从"黑屏等缓冲"变为"先看图再起播"。
2. **完播率信号回传**（前端 + 后端，2 天）：把 `watch_duration_ms` 与 `is_completed` 真正用于 M11 排序特征，形成"看得越久推得越多"的闭环。
3. **收藏态与收藏数**（后端，半天）：补 `FeedStat.favorite_count` 与 `FeedItem.is_favorited`，消除互动栏的空缺与 N+1。
4. **分享计数与分享卡**（后端 + 前端，1 天）：`POST /videos/:id/share` 落库 `share_count`，为传播路径分析提供数据。
5. **负反馈**（后端 + 前端，2 天）：`POST /videos/:id/feedback`（不感兴趣/减少推荐），写入曝光负样本，是推荐质量提升性价比最高的一项。
6. **评论楼中楼**（后端，2 天）：`parent_id` 过滤 + `reply_count`，配合"展开 N 条回复"。
7. **通知聚合**（后端，1 天）：同视频同类型 5 分钟内合并为一条，减少红点噪声。
8. **弱网策略**（前端，1 天）：`navigator.connection.effectiveType` 为 2g/3g 时只加载封面与文字，点击才起播。
9. **无障碍**（前端，1 天）：键盘全流程、焦点管理、`prefers-reduced-motion`，成本低但很少有人做。
10. **画中画与后台播放**（前端，1 天）：`requestPictureInPicture()` + Media Session API，桌面端体验加分。

---

## 十、核心术语表

| 术语 | 定义 |
|---|---|
| **场景流（Scene）** | 后端用独立路径区分内容来源（`/feed/recommend`、`/feed/following`、`/feed/global`、`/feed/hot`、`/feed/like-rank`），领域内以 `scene` 标识。前端 tab 与场景一一对应。 |
| **不透明游标** | 服务端发放的分页位置标记，形态可能是数字 ID 或 Base64 串，前端只负责原样回传。 |
| **软鉴权** | 带 token 则注入 `is_liked`/`is_followed`，不带 token 按游客返回，不返回 401。 |
| **乐观更新** | 先改本地 UI 再发请求，失败回滚；用于点赞、收藏、关注等高频道操作。 |
| **单飞刷新** | 多个请求同时 401 时，只发起一次 refresh，其余请求等待其结果后重放。 |
| **红心动画** | 双击点赞时在点击坐标处播放的心形粒子动效，`prefers-reduced-motion` 下退化为静态缩放。 |
| **演示账号** | 种子数据生成的固定账号，凭据明文写在 README 与登录页，仅用于本地验收。 |
