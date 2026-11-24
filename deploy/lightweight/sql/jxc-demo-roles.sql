-- 演示角色：9001 老板（JXC 全量）/ 9002 店长 / 9003 店员
-- 接口权限本身在 docs/sql/jxc-casbin.sql；这里只补 sys_authorities 行。
-- 幂等：可重复执行。
INSERT INTO sys_authorities
  (authority_id, authority_name, parent_id, data_scope, default_router, created_at, updated_at, deleted_at)
VALUES
  (9001, '老板', 0, 1, '', NOW(), NOW(), NULL),
  (9002, '店长', 0, 1, '', NOW(), NOW(), NULL),
  (9003, '店员', 0, 1, '', NOW(), NOW(), NULL)
ON DUPLICATE KEY UPDATE
  authority_name = VALUES(authority_name),
  updated_at = NOW();
