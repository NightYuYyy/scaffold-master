import type { HttpClient, QueryParams } from './http'
import type { IdResponse, PageResult } from './system'

/* ============================================================
   用户中心平台管理接口 /api/platform/*（契约见 docs/platform-api.md）。
   应用范围由后端按当前账号过滤：super_admin 全部，其他只看绑定的应用。
   ============================================================ */

/** 应用：code 编码（唯一）、allowRegister 是否开放注册、captchaEnabled 注册登录是否要验证码、tokenExpiresSeconds 用户令牌有效期（秒） */
export interface AppRow {
  id: number
  code: string
  name: string
  status: number
  allowRegister: number
  captchaEnabled: number
  tokenExpiresSeconds: number
  remark?: string
  userCount: number
  keyCount: number
  createTime: string
}

export interface AppWriteRequest {
  code?: string
  name?: string
  status?: number
  allowRegister?: number
  captchaEnabled?: number
  tokenExpiresSeconds?: number
  remark?: string
}

/** 应用筛选下拉选项（登录即可读取） */
export interface AppOption {
  id: number
  code: string
  name: string
}

/** 接口密钥：apiKey 公开标识，secretMasked 脱敏后的密钥，lastUsedTime 最近使用时间 */
export interface AppKeyRow {
  id: number
  name: string
  apiKey: string
  secretMasked: string
  status: number
  lastUsedTime?: string | null
  createTime: string
}

/** 新建密钥的返回：apiSecret 明文只在此处出现一次 */
export interface AppKeyCreated {
  id: number
  apiKey: string
  apiSecret: string
}

export type AppFieldType = 'string' | 'number' | 'bool' | 'date' | 'enum' | 'counter'

/** 扩展字段：options 枚举为字符串数组、数值账户为 { allowNegative, max }、其他为 null；userEditable 用户能否自己改 */
export interface AppFieldRow {
  id: number
  appId: number
  key: string
  label: string
  type: AppFieldType
  required: number
  userEditable: number
  defaultValue?: string | null
  options?: string[] | { allowNegative?: boolean; max?: number | null } | null
  sort: number
  status: number
  remark?: string
  createTime: string
}

export interface AppFieldWriteRequest {
  appId?: number
  key?: string
  label?: string
  type?: AppFieldType
  required?: number
  userEditable?: number
  defaultValue?: string | null
  options?: AppFieldRow['options']
  sort?: number
  status?: number
  remark?: string
}

/** 应用用户：profile 非 counter 字段值，counters 各数值账户余额 */
export interface AppUserRow {
  id: number
  appId: number
  username: string
  nickname: string
  avatar?: string | null
  status: number
  profile: Record<string, unknown>
  counters: Record<string, number>
  registerIp?: string
  lastLoginTime?: string | null
  createTime: string
}

export interface AppUserWriteRequest {
  appId?: number
  username?: string
  password?: string
  nickname?: string
  avatar?: string
  status?: number
  profile?: Record<string, unknown>
}

/** 账户加减：amount 非 0 数字（最多 4 位小数），reason 必填 */
export interface CounterAdjustRequest {
  amount: number
  reason: string
}

/** 账户加减结果：balance 调整后余额、logId 流水 ID、duplicated 是否命中幂等 */
export interface CounterAdjustResult {
  balance: number
  logId: number
  duplicated: boolean
}

/** 账户流水：balanceAfter 变动后余额、refNo 幂等号、operator 操作者（admin:<账号> 或应用后端） */
export interface CounterLogRow {
  id: number
  appId: number
  userId: number
  username: string
  fieldKey: string
  amount: number
  balanceAfter: number
  reason: string
  refNo: string
  operator: string
  createTime: string
}

/** 认证日志：event 事件（register / login / logout / refresh / login_failed / register_failed）、status 1 成功 0 失败、msg 说明 */
export interface AuthLogRow {
  id: number
  appId: number
  userId?: number | null
  username: string
  event: string
  status: number
  ip?: string
  msg?: string
  eventTime: string
}

export function createPlatformApi(client: HttpClient) {
  return {
    apps: {
      list: (query?: QueryParams) => client.get<PageResult<AppRow>>('/platform/apps', query),
      options: () => client.get<AppOption[]>('/platform/apps/options'),
      get: (id: number) => client.get<AppRow>(`/platform/apps/${id}`),
      create: (body: AppWriteRequest) => client.post<IdResponse>('/platform/apps', body),
      update: (id: number, body: AppWriteRequest) => client.put<void>(`/platform/apps/${id}`, body),
      updateStatus: (id: number, status: number) => client.put<void>(`/platform/apps/${id}/status`, { status }),
      remove: (id: number) => client.delete<void>(`/platform/apps/${id}`),
      export: (query?: QueryParams) => client.download('/platform/apps/export', query),
      keys: {
        list: (appId: number) => client.get<AppKeyRow[]>(`/platform/apps/${appId}/keys`),
        create: (appId: number, name: string) => client.post<AppKeyCreated>(`/platform/apps/${appId}/keys`, { name }),
        updateStatus: (appId: number, keyId: number, status: number) => client.put<void>(`/platform/apps/${appId}/keys/${keyId}/status`, { status }),
        remove: (appId: number, keyId: number) => client.delete<void>(`/platform/apps/${appId}/keys/${keyId}`),
      },
    },
    users: {
      list: (query?: QueryParams) => client.get<PageResult<AppUserRow>>('/platform/users', query),
      get: (id: number) => client.get<AppUserRow>(`/platform/users/${id}`),
      create: (body: AppUserWriteRequest) => client.post<IdResponse>('/platform/users', body),
      update: (id: number, body: AppUserWriteRequest) => client.put<void>(`/platform/users/${id}`, body),
      updateStatus: (id: number, status: number) => client.put<void>(`/platform/users/${id}/status`, { status }),
      resetPassword: (id: number, password: string) => client.put<void>(`/platform/users/${id}/password`, { password }),
      remove: (id: number) => client.delete<void>(`/platform/users/${id}`),
      adjustCounter: (id: number, key: string, body: CounterAdjustRequest) => client.post<CounterAdjustResult>(`/platform/users/${id}/counters/${encodeURIComponent(key)}/adjust`, body),
      export: (query?: QueryParams) => client.download('/platform/users/export', query),
    },
    fields: {
      list: (appId: number) => client.get<AppFieldRow[]>('/platform/fields', { appId }),
      create: (body: AppFieldWriteRequest) => client.post<IdResponse>('/platform/fields', body),
      update: (id: number, body: AppFieldWriteRequest) => client.put<void>(`/platform/fields/${id}`, body),
      remove: (id: number) => client.delete<void>(`/platform/fields/${id}`),
    },
    counterLogs: {
      list: (query?: QueryParams) => client.get<PageResult<CounterLogRow>>('/platform/counter-logs', query),
      export: (query?: QueryParams) => client.download('/platform/counter-logs/export', query),
    },
    authLogs: {
      list: (query?: QueryParams) => client.get<PageResult<AuthLogRow>>('/platform/auth-logs', query),
      export: (query?: QueryParams) => client.download('/platform/auth-logs/export', query),
    },
  }
}
