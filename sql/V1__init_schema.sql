-- 用户中心平台初始化脚本：运营控制台（sys_*）+ 应用与应用用户（app_*）+ 代码生成（gen_*）。
-- 幂等：全部 CREATE TABLE IF NOT EXISTS，可重复执行。

-- ========== 运营控制台 ==========

CREATE TABLE IF NOT EXISTS sys_user (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '后台账号 ID',
    username VARCHAR(64) NOT NULL COMMENT '登录账号',
    password VARCHAR(128) NOT NULL COMMENT '登录密码密文',
    nickname VARCHAR(64) NOT NULL COMMENT '姓名 / 昵称',
    phone VARCHAR(32) DEFAULT NULL COMMENT '手机号',
    email VARCHAR(128) DEFAULT NULL COMMENT '邮箱',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '账号状态：1 正常，0 停用',
    last_login_time DATETIME DEFAULT NULL COMMENT '最后登录时间',
    deleted TINYINT NOT NULL DEFAULT 0 COMMENT '删除标记：1 删除，0 正常',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sys_user_username (username),
    KEY idx_sys_user_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台账号（运营人员与应用管理员）';

CREATE TABLE IF NOT EXISTS sys_role (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '角色 ID',
    role_name VARCHAR(100) NOT NULL COMMENT '角色名称',
    role_code VARCHAR(64) NOT NULL COMMENT '角色编码',
    sort INT NOT NULL DEFAULT 0 COMMENT '排序号',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '角色状态：1 正常，0 停用',
    builtin TINYINT NOT NULL DEFAULT 0 COMMENT '是否内置角色：1 是，0 否',
    remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
    deleted TINYINT NOT NULL DEFAULT 0 COMMENT '删除标记：1 删除，0 正常',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sys_role_code (role_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台角色';

CREATE TABLE IF NOT EXISTS sys_menu (
    id BIGINT NOT NULL COMMENT '菜单 ID',
    parent_id BIGINT NOT NULL DEFAULT 0 COMMENT '父级菜单 ID',
    menu_name VARCHAR(100) NOT NULL COMMENT '菜单名称',
    menu_type VARCHAR(16) NOT NULL COMMENT '菜单类型：dir 目录，menu 菜单，btn 按钮',
    icon VARCHAR(64) DEFAULT NULL COMMENT '菜单图标',
    path VARCHAR(200) DEFAULT NULL COMMENT '前端路由路径',
    component VARCHAR(200) DEFAULT NULL COMMENT '前端组件路径',
    permission VARCHAR(128) DEFAULT NULL COMMENT '权限标识',
    sort INT NOT NULL DEFAULT 0 COMMENT '排序号',
    visible TINYINT NOT NULL DEFAULT 1 COMMENT '是否显示：1 显示，0 隐藏',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '菜单状态：1 正常，0 停用',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_sys_menu_parent (parent_id),
    KEY idx_sys_menu_permission (permission)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台菜单和按钮权限';

CREATE TABLE IF NOT EXISTS sys_user_role (
    user_id BIGINT NOT NULL COMMENT '后台账号 ID',
    role_id BIGINT NOT NULL COMMENT '角色 ID',
    PRIMARY KEY (user_id, role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台账号角色关联';

CREATE TABLE IF NOT EXISTS sys_role_menu (
    role_id BIGINT NOT NULL COMMENT '角色 ID',
    menu_id BIGINT NOT NULL COMMENT '菜单 ID',
    PRIMARY KEY (role_id, menu_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色菜单关联';

CREATE TABLE IF NOT EXISTS sys_user_app (
    user_id BIGINT NOT NULL COMMENT '后台账号 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    PRIMARY KEY (user_id, app_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台账号可管理的应用（超级管理员不受限）';

CREATE TABLE IF NOT EXISTS sys_dict_type (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '字典类型 ID',
    dict_name VARCHAR(100) NOT NULL COMMENT '字典类型名称',
    dict_type VARCHAR(100) NOT NULL COMMENT '字典类型编码',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '字典状态：1 正常，0 停用',
    remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sys_dict_type (dict_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='字典类型';

CREATE TABLE IF NOT EXISTS sys_dict_data (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '字典数据 ID',
    dict_type VARCHAR(100) NOT NULL COMMENT '字典类型编码',
    dict_label VARCHAR(100) NOT NULL COMMENT '字典项显示文本',
    dict_value VARCHAR(100) NOT NULL COMMENT '字典项实际值',
    sort INT NOT NULL DEFAULT 0 COMMENT '排序号',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '字典项状态：1 正常，0 停用',
    tone VARCHAR(32) DEFAULT NULL COMMENT '前端标签色调',
    is_default TINYINT NOT NULL DEFAULT 0 COMMENT '是否默认项：1 是，0 否',
    remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_sys_dict_data_type (dict_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='字典数据';

CREATE TABLE IF NOT EXISTS sys_config (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '参数 ID',
    config_name VARCHAR(100) NOT NULL COMMENT '参数名称',
    config_key VARCHAR(100) NOT NULL COMMENT '参数键名',
    config_value VARCHAR(500) NOT NULL COMMENT '参数键值',
    config_type CHAR(1) NOT NULL DEFAULT 'N' COMMENT '是否系统内置：Y 是，N 否',
    builtin TINYINT NOT NULL DEFAULT 0 COMMENT '是否内置参数：1 是，0 否',
    remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_sys_config_key (config_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='参数配置';

CREATE TABLE IF NOT EXISTS sys_oper_log (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '操作日志 ID',
    module VARCHAR(100) NOT NULL COMMENT '操作模块',
    action VARCHAR(64) NOT NULL COMMENT '操作类型',
    request_method VARCHAR(16) DEFAULT NULL COMMENT '请求方法',
    request_url VARCHAR(500) DEFAULT NULL COMMENT '请求地址',
    request_params TEXT COMMENT '请求参数（已脱敏）',
    oper_name VARCHAR(64) DEFAULT NULL COMMENT '操作人',
    ip VARCHAR(64) DEFAULT NULL COMMENT 'IP 地址',
    location VARCHAR(128) DEFAULT NULL COMMENT '操作地点',
    cost BIGINT DEFAULT NULL COMMENT '耗时毫秒',
    result TINYINT NOT NULL DEFAULT 1 COMMENT '操作结果：1 成功，0 失败',
    error_msg TEXT COMMENT '错误信息',
    oper_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
    PRIMARY KEY (id),
    KEY idx_sys_oper_log_time (oper_time),
    KEY idx_sys_oper_log_oper_name (oper_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台操作日志';

CREATE TABLE IF NOT EXISTS sys_login_log (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '登录日志 ID',
    username VARCHAR(64) DEFAULT NULL COMMENT '登录账号',
    ip VARCHAR(64) DEFAULT NULL COMMENT 'IP 地址',
    location VARCHAR(128) DEFAULT NULL COMMENT '登录地点',
    browser VARCHAR(128) DEFAULT NULL COMMENT '浏览器',
    os VARCHAR(128) DEFAULT NULL COMMENT '操作系统',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '登录状态：1 成功，0 失败',
    msg VARCHAR(500) DEFAULT NULL COMMENT '提示消息',
    login_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '登录时间',
    PRIMARY KEY (id),
    KEY idx_sys_login_log_time (login_time),
    KEY idx_sys_login_log_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台登录日志';

-- ========== 应用与应用用户 ==========

CREATE TABLE IF NOT EXISTS app_info (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '应用 ID',
    app_code VARCHAR(64) NOT NULL COMMENT '应用编码，唯一；也是用户 token 的 aud',
    app_name VARCHAR(100) NOT NULL COMMENT '应用名称',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1 启用，0 停用（停用后开放接口全部拒绝）',
    allow_register TINYINT NOT NULL DEFAULT 1 COMMENT '是否开放自助注册：1 是，0 否',
    captcha_enabled TINYINT NOT NULL DEFAULT 1 COMMENT '注册登录是否校验验证码：1 是，0 否',
    token_expires_seconds INT NOT NULL DEFAULT 604800 COMMENT '用户 token 有效期（秒），默认 7 天',
    remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
    deleted TINYINT NOT NULL DEFAULT 0 COMMENT '删除标记：1 删除，0 正常',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_info_code (app_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用（接入平台的业务项目）';

CREATE TABLE IF NOT EXISTS app_api_key (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '密钥 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    key_name VARCHAR(100) NOT NULL COMMENT '密钥名称（如 生产 / 测试）',
    api_key VARCHAR(64) NOT NULL COMMENT '公开标识，请求头 X-App-Key；也是用户 token 的 kid',
    api_secret VARCHAR(128) NOT NULL COMMENT '密钥，请求头 X-App-Secret；用于签发和验证用户 token',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1 启用，0 停用',
    last_used_time DATETIME DEFAULT NULL COMMENT '最近使用时间',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_api_key (api_key),
    KEY idx_app_api_key_app (app_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用接口密钥';

CREATE TABLE IF NOT EXISTS app_user (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '应用用户 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    username VARCHAR(64) NOT NULL COMMENT '登录账号，应用内唯一',
    password VARCHAR(128) DEFAULT NULL COMMENT '登录密码密文；仅第三方登录的用户可为空',
    nickname VARCHAR(64) NOT NULL DEFAULT '' COMMENT '昵称',
    avatar VARCHAR(500) DEFAULT NULL COMMENT '头像地址',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1 正常，0 停用',
    profile JSON DEFAULT NULL COMMENT '扩展字段值，键为 app_field.field_key',
    register_ip VARCHAR(64) DEFAULT NULL COMMENT '注册 IP',
    last_login_time DATETIME DEFAULT NULL COMMENT '最后登录时间',
    last_login_ip VARCHAR(64) DEFAULT NULL COMMENT '最后登录 IP',
    deleted TINYINT NOT NULL DEFAULT 0 COMMENT '删除标记：1 删除，0 正常',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建来源：register / admin:<账号> / api:<api_key>',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '注册时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_user_username (app_id, username),
    KEY idx_app_user_app_status (app_id, status),
    KEY idx_app_user_create (app_id, create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用用户（C 端用户，按应用隔离）';

CREATE TABLE IF NOT EXISTS app_user_identity (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '身份 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    user_id BIGINT NOT NULL COMMENT '应用用户 ID',
    provider VARCHAR(32) NOT NULL COMMENT '身份类型：phone / email / wechat …',
    identifier VARCHAR(191) NOT NULL COMMENT '手机号 / 邮箱 / openid',
    verified TINYINT NOT NULL DEFAULT 0 COMMENT '是否已验证：1 是，0 否',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_user_identity (app_id, provider, identifier),
    KEY idx_app_user_identity_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用用户登录身份（为手机号 / 邮箱 / 微信登录预留）';

CREATE TABLE IF NOT EXISTS app_field (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '字段 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    field_key VARCHAR(64) NOT NULL COMMENT '字段键，接口 profile 中的键名',
    label VARCHAR(100) NOT NULL COMMENT '字段名称',
    field_type VARCHAR(16) NOT NULL COMMENT '类型：string / number / bool / date / enum / counter',
    required TINYINT NOT NULL DEFAULT 0 COMMENT '注册时是否必填：1 是，0 否',
    user_editable TINYINT NOT NULL DEFAULT 0 COMMENT '用户可否通过 /me 修改：1 是，0 否',
    default_value VARCHAR(500) DEFAULT NULL COMMENT '默认值（文本形式）',
    options JSON DEFAULT NULL COMMENT 'enum 的可选值数组；counter 的 {"allowNegative":false,"max":null}',
    sort INT NOT NULL DEFAULT 0 COMMENT '排序号',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1 启用，0 停用',
    remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_field (app_id, field_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用扩展字段定义';

CREATE TABLE IF NOT EXISTS app_counter (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '账户 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    user_id BIGINT NOT NULL COMMENT '应用用户 ID',
    field_key VARCHAR(64) NOT NULL COMMENT '字段键（field_type = counter）',
    balance DECIMAL(20,4) NOT NULL DEFAULT 0 COMMENT '余额',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_counter (app_id, user_id, field_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用用户数值账户（积分 / 币等）';

CREATE TABLE IF NOT EXISTS app_counter_log (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '流水 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    user_id BIGINT NOT NULL COMMENT '应用用户 ID',
    field_key VARCHAR(64) NOT NULL COMMENT '字段键',
    amount DECIMAL(20,4) NOT NULL COMMENT '变动量，正为增加负为扣减',
    balance_after DECIMAL(20,4) NOT NULL COMMENT '变动后余额',
    reason VARCHAR(200) NOT NULL COMMENT '变动原因',
    ref_no VARCHAR(64) NOT NULL COMMENT '幂等号，应用内唯一',
    operator VARCHAR(64) NOT NULL COMMENT '操作方：api:<api_key> 或 admin:<后台账号>',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '发生时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_app_counter_log_ref (app_id, ref_no),
    KEY idx_app_counter_log_user (app_id, user_id, field_key, id),
    KEY idx_app_counter_log_time (app_id, create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数值账户流水';

CREATE TABLE IF NOT EXISTS app_auth_log (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '日志 ID',
    app_id BIGINT NOT NULL COMMENT '应用 ID',
    user_id BIGINT DEFAULT NULL COMMENT '应用用户 ID（失败时可为空）',
    username VARCHAR(64) DEFAULT NULL COMMENT '登录账号',
    event VARCHAR(32) NOT NULL COMMENT '事件：register / login / logout / refresh / login_failed / register_failed',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '结果：1 成功，0 失败',
    ip VARCHAR(64) DEFAULT NULL COMMENT 'IP 地址',
    msg VARCHAR(500) DEFAULT NULL COMMENT '提示消息',
    event_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '发生时间',
    PRIMARY KEY (id),
    KEY idx_app_auth_log_app_time (app_id, event_time),
    KEY idx_app_auth_log_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用用户认证日志';

-- ========== 代码生成（开发工具，默认不在菜单中） ==========

CREATE TABLE IF NOT EXISTS gen_table (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '代码生成表 ID',
    table_name VARCHAR(100) NOT NULL COMMENT '数据库表名',
    table_comment VARCHAR(200) DEFAULT NULL COMMENT '表说明',
    class_name VARCHAR(100) NOT NULL COMMENT '类名',
    module_name VARCHAR(64) NOT NULL COMMENT '模块名称',
    business_name VARCHAR(64) NOT NULL COMMENT '业务名称',
    synced TINYINT NOT NULL DEFAULT 0 COMMENT '是否同步数据库字段：1 是，0 否',
    create_by VARCHAR(64) DEFAULT NULL COMMENT '创建人',
    update_by VARCHAR(64) DEFAULT NULL COMMENT '更新人',
    create_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    update_time DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_gen_table_name (table_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='代码生成表配置';

CREATE TABLE IF NOT EXISTS gen_table_column (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '代码生成字段 ID',
    table_id BIGINT NOT NULL COMMENT '代码生成表 ID',
    column_name VARCHAR(100) NOT NULL COMMENT '数据库字段名',
    java_field VARCHAR(100) NOT NULL COMMENT '属性名（小驼峰）',
    java_type VARCHAR(64) NOT NULL COMMENT '字段类型标识',
    jdbc_type VARCHAR(64) NOT NULL COMMENT '数据库类型',
    column_comment VARCHAR(200) DEFAULT NULL COMMENT '字段说明',
    is_insert TINYINT NOT NULL DEFAULT 1 COMMENT '是否新增字段',
    is_edit TINYINT NOT NULL DEFAULT 1 COMMENT '是否编辑字段',
    is_list TINYINT NOT NULL DEFAULT 1 COMMENT '是否列表展示',
    is_query TINYINT NOT NULL DEFAULT 0 COMMENT '是否查询字段',
    query_type VARCHAR(32) NOT NULL DEFAULT '=' COMMENT '查询方式',
    form_type VARCHAR(64) NOT NULL DEFAULT '文本框' COMMENT '表单控件类型',
    required TINYINT NOT NULL DEFAULT 0 COMMENT '是否必填',
    sort INT NOT NULL DEFAULT 0 COMMENT '排序号',
    PRIMARY KEY (id),
    KEY idx_gen_table_column_table (table_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='代码生成字段配置';
