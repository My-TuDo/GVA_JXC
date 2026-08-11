-- ============================================================
-- AI 智能助手：sys_apis / casbin / 菜单 一次性注册（幂等可重跑）
-- 背景：AI 助手（/jxc/ai/analyze、/jxc/ai/recommend）为新增功能，
--       路由挂 PrivateGroup 走完整鉴权链，未注册权限前所有角色 403。
-- 执行（Windows 下必须指定 utf8mb4，否则中文 description 报 1366 错）：
--   mysql -h127.0.0.1 -uroot -p --default-character-set=utf8mb4 gva < ai-register.sql
-- 矩阵：
--   老板 888 / 店长 9001：AI 业绩分析 + 商品推荐（全库口径 + 菜单门禁）
--   销售 9002 / 采购 9003 / 仓管 9004：不配置（默认不可用，可按需在
--   「系统管理 → 角色管理」菜单中手动分配）
-- 菜单说明：挂在 jxc 分组下（name='jxc'），Hidden=1 不显示在侧边栏，
--   仅作为权限门禁（前端据此显示悬浮入口）；找不到 jxc 分组时挂到根。
-- 同步：路由挂载见 initialize/router_biz.go；前端组件见 web/src/view/jxc/ai/
-- ============================================================

-- ========== 1. sys_apis 字典注册（API 管理界面可视化） ==========

INSERT INTO `sys_apis` (`path`, `description`, `api_group`, `method`, `created_at`, `updated_at`)
SELECT '/jxc/ai/analyze', 'AI 销售业绩分析（流式）', 'jxcAI', 'POST', NOW(3), NOW(3) FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `sys_apis` WHERE path='/jxc/ai/analyze' AND method='POST' AND deleted_at IS NULL);

INSERT INTO `sys_apis` (`path`, `description`, `api_group`, `method`, `created_at`, `updated_at`)
SELECT '/jxc/ai/recommend', 'AI 客户需求商品推荐（流式）', 'jxcAI', 'POST', NOW(3), NOW(3) FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `sys_apis` WHERE path='/jxc/ai/recommend' AND method='POST' AND deleted_at IS NULL);

-- ========== 2. casbin 接口权限（老板 888 / 店长 9001） ==========

INSERT INTO `casbin_rule` (`ptype`, `v0`, `v1`, `v2`)
SELECT 'p', '888', '/jxc/ai/analyze', 'POST' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `casbin_rule` WHERE ptype='p' AND v0='888' AND v1='/jxc/ai/analyze' AND v2='POST');
INSERT INTO `casbin_rule` (`ptype`, `v0`, `v1`, `v2`)
SELECT 'p', '888', '/jxc/ai/recommend', 'POST' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `casbin_rule` WHERE ptype='p' AND v0='888' AND v1='/jxc/ai/recommend' AND v2='POST');

INSERT INTO `casbin_rule` (`ptype`, `v0`, `v1`, `v2`)
SELECT 'p', '9001', '/jxc/ai/analyze', 'POST' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `casbin_rule` WHERE ptype='p' AND v0='9001' AND v1='/jxc/ai/analyze' AND v2='POST');
INSERT INTO `casbin_rule` (`ptype`, `v0`, `v1`, `v2`)
SELECT 'p', '9001', '/jxc/ai/recommend', 'POST' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `casbin_rule` WHERE ptype='p' AND v0='9001' AND v1='/jxc/ai/recommend' AND v2='POST');

-- ========== 3. 菜单（AI 智能助手，Hidden=1 仅作权限门禁） ==========

INSERT INTO `sys_base_menus` (`parent_id`, `path`, `name`, `component`, `hidden`, `sort`, `menu_level`, `title`, `icon`, `created_at`, `updated_at`)
SELECT CASE WHEN (SELECT id FROM `sys_base_menus` WHERE name='jxc' AND deleted_at IS NULL ORDER BY id LIMIT 1) IS NOT NULL
            THEN (SELECT id FROM `sys_base_menus` WHERE name='jxc' AND deleted_at IS NULL ORDER BY id LIMIT 1)
            ELSE 0 END,
       'aiAssistant', 'aiAssistant', 'view/jxc/ai/index.vue', 1, 99,
       CASE WHEN (SELECT id FROM `sys_base_menus` WHERE name='jxc' AND deleted_at IS NULL ORDER BY id LIMIT 1) IS NOT NULL THEN 1 ELSE 0 END,
       'AI 智能助手', 'sparkles-gva', NOW(3), NOW(3) FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `sys_base_menus` WHERE name='aiAssistant' AND deleted_at IS NULL);

-- 菜单授权：老板 888 / 店长 9001
INSERT INTO `sys_authority_menus` (`sys_base_menu_id`, `sys_authority_authority_id`)
SELECT m.id, '888' FROM `sys_base_menus` m WHERE m.name='aiAssistant' AND m.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM `sys_authority_menus` WHERE sys_base_menu_id=m.id AND sys_authority_authority_id='888');
INSERT INTO `sys_authority_menus` (`sys_base_menu_id`, `sys_authority_authority_id`)
SELECT m.id, '9001' FROM `sys_base_menus` m WHERE m.name='aiAssistant' AND m.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM `sys_authority_menus` WHERE sys_base_menu_id=m.id AND sys_authority_authority_id='9001');
