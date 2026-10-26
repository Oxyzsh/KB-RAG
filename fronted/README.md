# fronted

KB-RAG 的 Vue 3 + TypeScript 前端。

## 开发

先启动 Go API（默认 `127.0.0.1:8080`），再运行：

```bash
npm install
npm run dev
```

Vite 会把 `/api` 代理到 `http://127.0.0.1:8080`。

## 构建

```bash
npm run type-check
npm run build
```

产物写入 `fronted/dist`，由 `KB-RAG serve` 从磁盘托管。`dist` 与 `node_modules` 不提交。

## 功能

- 登录：租户 slug、用户名、密码；令牌保存在 `sessionStorage`，有效期 12 小时；
- 超级管理员可在侧栏切换租户，切换后各页面自动刷新为目标租户数据；
- 问答：流式输出、停止生成、答案 markdown-it 渲染（关闭原始 HTML）；引用片段以文本展示；
- 知识库：上传进入异步任务队列，展示处理阶段并支持取消；统计、文档列表、重试、删除（仅 admin/editor 可见写操作）；
- 系统管理（admin）：用户管理、审计日志；超级管理员额外有租户管理与全局审计。

## 构建优化

Element Plus 组件按需引入，页面路由懒加载；生产构建按路由与公共依赖分包，避免单个大包。

## 说明

- 前端不保存 LLM API Key，只保存会话令牌；
- 401 自动清理令牌并回登录页；403 只提示权限不足；
- 页面刷新后通过 `GET /api/session` 恢复当前用户。
