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

## 说明

- 令牌保存在 `sessionStorage`；
- 答案使用 markdown-it，关闭原始 HTML；
- 只读引用片段，不渲染片段内的 HTML。
