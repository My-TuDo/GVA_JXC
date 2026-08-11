# AI 智能体窗口（智能助手）

## 基本信息

- 提出日期：2026-08-11
- 当前状态：`active`
- 需求类型：新功能（PC 管理端 AI 助手）
- 优先级：高
- 需求文件：`docs/11-AI智能体窗口.md`（已批准实施计划）

## 用户原始意图摘要

为进销存系统增加一个「智能体窗口」：调用外部 AI 能力（DeepSeek 官方 API），可①分析销售业绩 ②输入一段客户需求后推荐购买什么商品。

## 影响范围

- 后端：`server/config/ai.go`（ai 配置段）、`server/service/jxc/ai.go` / `ai_llm.go`（AI Service + LLM 客户端）、`server/api/v1/jxc/ai.go`（analyze/recommend SSE 接口）、`server/router/jxc/ai.go`（挂 PrivateGroup）、`initialize/router_biz.go`、`docs/sql/ai-register.sql`（sys_apis + casbin + 菜单种子）
- 前端：`web/src/api/jxc/ai.js`（SSE 封装）、`web/src/view/jxc/ai/AiAssistant.vue`（对话组件）+ `index.vue`（独立页）、看板页/收银台页悬浮入口、`web/src/assets/icons/sparkles-gva.svg`
- 文档：`docs/11-AI智能体窗口.md`
- 配置：`config.yaml` / `config.docker.yaml` 新增 `ai.llm` 段（apiKey 支持环境变量 `GVA_AI_APIKEY`）

## 涉及对象

- 模块：jxc（AI 助手子模块）
- 接口：`POST /jxc/ai/analyze`（SSE 流式业绩分析）、`POST /jxc/ai/recommend`（先 candidates 事件再流式推荐语）
- 页面：经营看板（右上 AI 助手悬浮按钮，快捷提问）、收银台（tab 栏「AI 助手」按钮，推荐候选可加购）
- 配置：`ai.llm.{base_url, api_key, model, timeout, max_tokens, temperature}`

## 已确认约束

- PC 管理端优先（小程序不做，后续可复用后端接口）
- LLM：DeepSeek 官方 API（OpenAI 兼容 chat/completions）
- 推荐路线：**结构化条件检索**（LLM JSON 解析需求 → 现有商品查询过滤 → LLM 生成推荐语），不做 RAG/向量库
- 数据口径：**全库口径 + RBAC 菜单门禁**（与看板一致：谁能用由角色菜单控制，无权限角色 403）
- AI 零写操作：不提供任何写工具，只读聚合 + 只读检索
- 防引导性语言：System Prompt 三层结构（角色/硬约束/<data> 数据隔离），硬防线 = 无写工具 + Casbin 门禁
- 权限：默认老板 888 / 店长 9001 可用，其他角色按需在角色管理分配
- 流式输出：前端 SSE（fetch + ReadableStream，复用 autoCode.js 风格）；后端 SSE 透传 DeepSeek 增量

## 当前进展

- 2026-08-11：计划已批准（AskUserQuestion 确认 4 项决策），分支 `feature/jxc-ai-assistant` 开发中
- 2026-08-11：后端 M1 完成（配置/LLM客户端/Service/API/路由/权限 SQL），13 个单测 + go vet 通过
- 2026-08-11：前端 M2 完成（SSE 封装/对话组件/看板+收银台入口），vite build 通过
- 2026-08-11：文档（docs/11 + 配置说明）与记忆登记完成

## 后续待办

- [ ] 执行 `docs/sql/ai-register.sql`（权限初始化，需连真实 MySQL）
- [ ] 配置 DeepSeek API Key（config.yaml 或环境变量）
- [ ] 联调验收（M3）：真实 API 跑通分析流式 + 推荐加购；权限 403 验证；prompt injection 拒绝验证
- [ ] 收尾：`feature/jxc-ai-assistant` 合并 develop

## 更新规则

- 本文件只承载「AI 智能体窗口」这一个功能点
- 后续如拆分新功能点（如小程序 AI 入口、RAG 升级），按 AGENT.MD 规则新建独立记忆文件
