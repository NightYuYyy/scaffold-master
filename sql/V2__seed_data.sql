-- 初始数据：后台角色与账号、菜单与权限、字典、参数，以及一个示例应用（仅供本地联调，接入真实环境前请删除或更换密钥）。
-- 所有后台账号与示例应用用户的开发密码均为 admin123。

INSERT IGNORE INTO sys_role (id, role_name, role_code, sort, status, builtin, remark, create_by, update_by)
VALUES
    (1, '超级管理员', 'super_admin', 1, 1, 1, '平台最高权限，可见全部应用，不可删除', 'system', 'system'),
    (2, '平台管理员', 'platform_admin', 2, 1, 0, '管理全部应用、应用用户与后台配置', 'system', 'system'),
    (3, '应用管理员', 'app_admin', 3, 1, 0, '只能管理「后台账号」里绑定的应用：用户、字段、密钥与日志', 'system', 'system'),
    (4, '审计专员', 'auditor', 4, 1, 0, '只读查看操作日志、登录日志与认证日志', 'system', 'system');

INSERT IGNORE INTO sys_user (id, username, password, nickname, phone, email, status, create_by, update_by)
VALUES
    (1, 'admin', '$2b$10$L7q28dpKkcfrrUzfvZebqe7pvxcKpaSgh7EUenf9Q5z2LiYKxDpIu', '平台管理员', '', 'admin@example.com', 1, 'system', 'system'),
    (2, 'appadmin', '$2b$10$L7q28dpKkcfrrUzfvZebqe7pvxcKpaSgh7EUenf9Q5z2LiYKxDpIu', '示例应用管理员', '', 'appadmin@example.com', 1, 'system', 'system');

INSERT IGNORE INTO sys_user_role (user_id, role_id) VALUES (1, 1), (2, 3);

INSERT IGNORE INTO sys_menu (id, parent_id, menu_name, menu_type, icon, path, component, permission, sort, visible, status, create_by, update_by)
VALUES
    (1, 0, '工作台', 'menu', 'dashboard', '/dashboard', 'dashboard/index', '', 1, 1, 1, 'system', 'system'),

    (100, 0, '应用平台', 'dir', 'layers', '/platform', '', '', 2, 1, 1, 'system', 'system'),
    (101, 100, '应用管理', 'menu', 'grid', '/platform/app', 'platform/app/index', 'platform:app:list', 1, 1, 1, 'system', 'system'),
    (1011, 101, '新增应用', 'btn', '', '', '', 'platform:app:add', 1, 1, 1, 'system', 'system'),
    (1012, 101, '编辑应用', 'btn', '', '', '', 'platform:app:edit', 2, 1, 1, 'system', 'system'),
    (1013, 101, '删除应用', 'btn', '', '', '', 'platform:app:remove', 3, 1, 1, 'system', 'system'),
    (1014, 101, '查看密钥', 'btn', '', '', '', 'platform:appkey:list', 4, 1, 1, 'system', 'system'),
    (1015, 101, '新增密钥', 'btn', '', '', '', 'platform:appkey:add', 5, 1, 1, 'system', 'system'),
    (1016, 101, '停用删除密钥', 'btn', '', '', '', 'platform:appkey:remove', 6, 1, 1, 'system', 'system'),
    (102, 100, '应用用户', 'menu', 'users', '/platform/user', 'platform/user/index', 'platform:user:list', 2, 1, 1, 'system', 'system'),
    (1021, 102, '新增用户', 'btn', '', '', '', 'platform:user:add', 1, 1, 1, 'system', 'system'),
    (1022, 102, '编辑用户', 'btn', '', '', '', 'platform:user:edit', 2, 1, 1, 'system', 'system'),
    (1023, 102, '删除用户', 'btn', '', '', '', 'platform:user:remove', 3, 1, 1, 'system', 'system'),
    (1024, 102, '重置密码', 'btn', '', '', '', 'platform:user:resetPwd', 4, 1, 1, 'system', 'system'),
    (1025, 102, '账户调整', 'btn', '', '', '', 'platform:counter:adjust', 5, 1, 1, 'system', 'system'),
    (103, 100, '字段配置', 'menu', 'sliders', '/platform/field', 'platform/field/index', 'platform:field:list', 3, 1, 1, 'system', 'system'),
    (1031, 103, '新增字段', 'btn', '', '', '', 'platform:field:add', 1, 1, 1, 'system', 'system'),
    (1032, 103, '编辑字段', 'btn', '', '', '', 'platform:field:edit', 2, 1, 1, 'system', 'system'),
    (1033, 103, '删除字段', 'btn', '', '', '', 'platform:field:remove', 3, 1, 1, 'system', 'system'),
    (104, 100, '账户流水', 'menu', 'activity', '/platform/counter-log', 'platform/counterlog/index', 'platform:counter:list', 4, 1, 1, 'system', 'system'),
    (105, 100, '认证日志', 'menu', 'lock', '/platform/auth-log', 'platform/authlog/index', 'platform:authlog:list', 5, 1, 1, 'system', 'system'),

    (200, 0, '系统管理', 'dir', 'gear', '/system', '', '', 3, 1, 1, 'system', 'system'),
    (201, 200, '后台账号', 'menu', 'user', '/system/user', 'system/user/index', 'system:user:list', 1, 1, 1, 'system', 'system'),
    (2011, 201, '新增账号', 'btn', '', '', '', 'system:user:add', 1, 1, 1, 'system', 'system'),
    (2012, 201, '编辑账号', 'btn', '', '', '', 'system:user:edit', 2, 1, 1, 'system', 'system'),
    (2013, 201, '删除账号', 'btn', '', '', '', 'system:user:remove', 3, 1, 1, 'system', 'system'),
    (2014, 201, '重置密码', 'btn', '', '', '', 'system:user:resetPwd', 4, 1, 1, 'system', 'system'),
    (202, 200, '角色管理', 'menu', 'role', '/system/role', 'system/role/index', 'system:role:list', 2, 1, 1, 'system', 'system'),
    (2021, 202, '新增角色', 'btn', '', '', '', 'system:role:add', 1, 1, 1, 'system', 'system'),
    (2022, 202, '编辑角色', 'btn', '', '', '', 'system:role:edit', 2, 1, 1, 'system', 'system'),
    (2023, 202, '删除角色', 'btn', '', '', '', 'system:role:remove', 3, 1, 1, 'system', 'system'),
    (203, 200, '菜单管理', 'menu', 'menu', '/system/menu', 'system/menu/index', 'system:menu:list', 3, 1, 1, 'system', 'system'),
    (2031, 203, '新增菜单', 'btn', '', '', '', 'system:menu:add', 1, 1, 1, 'system', 'system'),
    (2032, 203, '编辑菜单', 'btn', '', '', '', 'system:menu:edit', 2, 1, 1, 'system', 'system'),
    (2033, 203, '删除菜单', 'btn', '', '', '', 'system:menu:remove', 3, 1, 1, 'system', 'system'),
    (204, 200, '字典管理', 'menu', 'dict', '/system/dict', 'system/dict/index', 'system:dict:list', 4, 1, 1, 'system', 'system'),
    (2041, 204, '新增字典', 'btn', '', '', '', 'system:dict:add', 1, 1, 1, 'system', 'system'),
    (2042, 204, '编辑字典', 'btn', '', '', '', 'system:dict:edit', 2, 1, 1, 'system', 'system'),
    (2043, 204, '删除字典', 'btn', '', '', '', 'system:dict:remove', 3, 1, 1, 'system', 'system'),
    (205, 200, '参数配置', 'menu', 'gear', '/system/config', 'system/config/index', 'system:config:list', 5, 1, 1, 'system', 'system'),
    (2051, 205, '新增参数', 'btn', '', '', '', 'system:config:add', 1, 1, 1, 'system', 'system'),
    (2052, 205, '编辑参数', 'btn', '', '', '', 'system:config:edit', 2, 1, 1, 'system', 'system'),
    (2053, 205, '删除参数', 'btn', '', '', '', 'system:config:remove', 3, 1, 1, 'system', 'system'),

    (300, 0, '系统监控', 'dir', 'log', '/monitor', '', '', 4, 1, 1, 'system', 'system'),
    (301, 300, '操作日志', 'menu', 'file', '/monitor/oper-log', 'monitor/operlog/index', 'monitor:operlog:list', 1, 1, 1, 'system', 'system'),
    (3011, 301, '清理操作日志', 'btn', '', '', '', 'monitor:operlog:remove', 1, 1, 1, 'system', 'system'),
    (302, 300, '登录日志', 'menu', 'lock', '/monitor/login-log', 'monitor/loginlog/index', 'monitor:loginlog:list', 2, 1, 1, 'system', 'system'),
    (3021, 302, '清理登录日志', 'btn', '', '', '', 'monitor:loginlog:remove', 1, 1, 1, 'system', 'system');

-- 代码生成是开发工具，默认不出现在菜单；需要时执行：
-- INSERT INTO sys_menu VALUES (400, 0, '系统工具', 'dir', 'tool', '/tool', '', '', 9, 1, 1, 'system', 'system', NOW(), NOW()),
--   (401, 400, '代码生成', 'menu', 'command', '/tool/generator', 'tool/gen/index', 'tool:gen:list', 1, 1, 1, 'system', 'system', NOW(), NOW());

-- 超级管理员：全部菜单；平台管理员：应用平台 + 系统监控 + 工作台；应用管理员：工作台 + 应用平台（不含新增 / 删除应用）；审计：工作台 + 系统监控 + 认证日志
INSERT IGNORE INTO sys_role_menu (role_id, menu_id) SELECT 1, id FROM sys_menu;
INSERT IGNORE INTO sys_role_menu (role_id, menu_id) SELECT 2, id FROM sys_menu WHERE id = 1 OR id BETWEEN 100 AND 199 OR id BETWEEN 300 AND 399 OR id BETWEEN 3000 AND 3999 OR id BETWEEN 1000 AND 1999;
INSERT IGNORE INTO sys_role_menu (role_id, menu_id) SELECT 3, id FROM sys_menu WHERE id = 1 OR ((id BETWEEN 100 AND 199 OR id BETWEEN 1000 AND 1999) AND id NOT IN (1011, 1013));
INSERT IGNORE INTO sys_role_menu (role_id, menu_id) SELECT 4, id FROM sys_menu WHERE id IN (1, 100, 105, 300, 301, 302);

INSERT IGNORE INTO sys_dict_type (id, dict_name, dict_type, status, remark, create_by, update_by)
VALUES
    (1, '系统开关', 'sys_normal_disable', 1, '正常 / 停用', 'system', 'system'),
    (2, '是否', 'sys_yes_no', 1, '通用是 / 否选项', 'system', 'system'),
    (3, '字段类型', 'app_field_type', 1, '应用扩展字段类型', 'system', 'system');

INSERT IGNORE INTO sys_dict_data (id, dict_type, dict_label, dict_value, sort, status, tone, is_default, create_by, update_by)
VALUES
    (1, 'sys_normal_disable', '正常', '1', 1, 1, 'ok', 1, 'system', 'system'),
    (2, 'sys_normal_disable', '停用', '0', 2, 1, 'danger', 0, 'system', 'system'),
    (3, 'sys_yes_no', '是', 'Y', 1, 1, 'ok', 1, 'system', 'system'),
    (4, 'sys_yes_no', '否', 'N', 2, 1, 'neutral', 0, 'system', 'system'),
    (5, 'app_field_type', '文本', 'string', 1, 1, 'info', 1, 'system', 'system'),
    (6, 'app_field_type', '数字', 'number', 2, 1, 'info', 0, 'system', 'system'),
    (7, 'app_field_type', '布尔', 'bool', 3, 1, 'info', 0, 'system', 'system'),
    (8, 'app_field_type', '日期', 'date', 4, 1, 'info', 0, 'system', 'system'),
    (9, 'app_field_type', '枚举', 'enum', 5, 1, 'purple', 0, 'system', 'system'),
    (10, 'app_field_type', '数值账户', 'counter', 6, 1, 'warn', 0, 'system', 'system');

INSERT IGNORE INTO sys_config (id, config_name, config_key, config_value, config_type, builtin, remark, create_by, update_by)
VALUES
    (1, '后台账号-初始密码', 'sys.user.initPassword', 'Yy@123456', 'Y', 1, '新建后台账号的默认密码', 'system', 'system'),
    (2, '后台登录-验证码开关', 'sys.account.captchaEnabled', 'true', 'Y', 1, '是否开启后台登录验证码', 'system', 'system'),
    (3, '主框架-默认外观', 'sys.index.appearance', 'light', 'N', 0, '可选 light / dark / auto', 'system', 'system'),
    (4, '登录告警-失败阈值', 'sys.login.alertThreshold', '5', 'N', 0, '同一 IP / 账号登录失败达到该次数列为异常', 'system', 'system'),
    (5, '应用用户-初始密码', 'app.user.initPassword', 'Aa@123456', 'Y', 1, '后台新建或重置应用用户密码时的默认值', 'system', 'system');

-- ========== 示例应用（本地联调用；接入真实环境前请删除或更换密钥） ==========

INSERT IGNORE INTO app_info (id, app_code, app_name, status, allow_register, captcha_enabled, token_expires_seconds, remark, create_by, update_by)
VALUES (1, 'demo', '示例应用', 1, 1, 1, 604800, '本地联调示例，演示注册登录、扩展字段与数值账户', 'system', 'system');

INSERT IGNORE INTO app_api_key (id, app_id, key_name, api_key, api_secret, status, create_by, update_by)
VALUES (1, 1, '本地联调', 'demo_pk_local', 'demo-secret-change-me-0123456789abcdef0123456789', 1, 'system', 'system');

INSERT IGNORE INTO sys_user_app (user_id, app_id) VALUES (2, 1);

INSERT IGNORE INTO app_field (id, app_id, field_key, label, field_type, required, user_editable, default_value, options, sort, status, remark, create_by, update_by)
VALUES
    (1, 1, 'coins', '积分', 'counter', 0, 0, '0', '{"allowNegative": false}', 1, 1, '签到、消费等场景的数值账户', 'system', 'system'),
    (2, 1, 'level', '等级', 'enum', 0, 0, 'bronze', '["bronze", "silver", "gold"]', 2, 1, '由业务后端调整', 'system', 'system'),
    (3, 1, 'inviteCode', '邀请码', 'string', 0, 1, NULL, NULL, 3, 1, '用户注册或个人中心可填写', 'system', 'system'),
    (4, 1, 'birthday', '生日', 'date', 0, 1, NULL, NULL, 4, 1, '', 'system', 'system');

INSERT IGNORE INTO app_user (id, app_id, username, password, nickname, status, profile, create_by, update_by)
VALUES (1, 1, 'demo_user', '$2b$10$L7q28dpKkcfrrUzfvZebqe7pvxcKpaSgh7EUenf9Q5z2LiYKxDpIu', '示例用户', 1, '{"level": "bronze", "inviteCode": "WELCOME"}', 'system', 'system');

INSERT IGNORE INTO app_counter (id, app_id, user_id, field_key, balance) VALUES (1, 1, 1, 'coins', 100);

INSERT IGNORE INTO app_counter_log (id, app_id, user_id, field_key, amount, balance_after, reason, ref_no, operator)
VALUES (1, 1, 1, 'coins', 100, 100, '示例数据：注册赠送', 'seed-welcome-1', 'admin:system');
