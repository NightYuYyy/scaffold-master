import type { HttpClient, QueryParams } from './http'
import type { PageResult } from './system'

/** 工作台概览读数：key 指标键、label 中文名、value 数值、sub 补充说明；delta 环比百分比、up 方向、fresh 上一周期为 0 */
export interface DashboardStat {
  key: string
  label: string
  value: number
  sub?: string
  delta?: number
  up?: boolean
  fresh?: boolean
}

/** 工作台概览（/dashboard/summary）：trend 注册 / 登录趋势，apps 各应用读数，activities 最近认证动态，counters 数值账户汇总 */
export interface DashboardSummary {
  stats: DashboardStat[]
  range: '24h' | '7d' | '30d'
  trend: { labels: string[]; registrations: number[]; logins: number[] }
  apps: Array<{ id: number; code: string; name: string; users: number; registrations: number; logins: number }>
  activities: Array<{ app: string; who: string; action: string; time: string; tint: 'ok' | 'danger' | string }>
  counters: Array<{ app: string; fieldKey: string; label: string; total: number; changes: number }>
}

export interface GeneratorFile {
  name: string
  path: string
  code: string
}

export interface GeneratorPreview {
  files: GeneratorFile[]
  activeFile: string
  code: string
}

export interface GeneratorTableUpdate {
  tableComment?: string
  synced?: boolean
}

export interface GeneratorImportRequest {
  tableNames: string[]
}

/** 服务健康：status 运行状态（UP 正常）、time 服务端时间 */
export interface HealthStatus {
  status: string
  time: string
}

export function createPortalApi(client: HttpClient) {
  return {
    health: () => client.get<HealthStatus>('/health'),
    dashboard: {
      summary: (query?: QueryParams) => client.get<DashboardSummary>('/dashboard/summary', query),
    },
    generator: {
      tables: {
        list: (query?: QueryParams) => client.get<PageResult<Record<string, unknown>>>('/tool/generator/tables', query),
        importable: (query?: QueryParams) => client.get<Record<string, unknown>[]>('/tool/generator/importable-tables', query),
        import: (body: GeneratorImportRequest) => client.post<Record<string, unknown>>('/tool/generator/import', body),
        columns: (id: number) => client.get<Record<string, unknown>[]>(`/tool/generator/tables/${id}/columns`),
        preview: (id: number) => client.get<GeneratorPreview>(`/tool/generator/tables/${id}/preview`),
        update: (id: number, body: GeneratorTableUpdate) => client.put<void>(`/tool/generator/tables/${id}`, body),
        generate: (id: number) => client.post<Record<string, unknown>>(`/tool/generator/tables/${id}/generate`),
        download: (id: number) => client.download(`/tool/generator/tables/${id}/download`),
      },
    },
  }
}
