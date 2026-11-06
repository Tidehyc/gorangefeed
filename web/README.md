# Web 客户端

GoRangeFeed 的移动端优先 Web 客户端，覆盖「刷 → 赞 → 评 → 藏 → 关注 → 拍 → 管」全链路。

## 技术栈

React 18 + Vite 5 + TypeScript (strict) + Tailwind CSS 3 + Zustand + React Router 6。

## 目录

```
web/
  index.html           Vite 入口
  legacy/index.html    M10 单文件演示页归档，不参与构建
  src/
    api/               接口封装（统一信封、单飞刷新、分片上传）
    types/             与后端 DTO 一一对应的类型
    stores/            auth / toast 状态
    hooks/             useCursorList 游标分页状态机
    lib/               格式化、Feed/Detail 结构归一
    components/        视频卡片、评论抽屉、骨架、头像等
    pages/             12 个页面
```

## 开发与构建

```bash
# 开发态（Vite 监听 :5173，代理 /api、/media 到 127.0.0.1:8080）
npm run dev            # 或 make web-dev

# 生产构建（产物落到 web/dist，由 Go 侧同源托管）
npm run build          # 或 make web-build
```

需求与验收口径见 [`docs/FRONTEND_REQUIREMENTS.md`](../docs/FRONTEND_REQUIREMENTS.md)。
