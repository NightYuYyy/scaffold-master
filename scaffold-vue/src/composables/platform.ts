import { computed, ref, watch } from 'vue'
import { platformApi, type AppFieldRow, type AppOption } from '@/api'
import { sessionVersion } from './auth'
import { toastError } from './toast'

/* ============================================================
   平台页面共用：可见应用列表（按账号范围由后端过滤）与「当前应用」选择。
   选中的应用按页面记在本机；账号切换后清空。
   ============================================================ */
export const appOptions = ref<AppOption[]>([])
let loaded: Promise<AppOption[]> | null = null

export function ensureAppOptions(force = false) {
  if (!force && loaded) return loaded
  const version = sessionVersion.value
  loaded = platformApi.apps.options()
    .then((rows) => {
      if (version === sessionVersion.value) appOptions.value = rows
      return rows
    })
    .catch((error) => {
      loaded = null
      toastError(error, '应用列表加载失败')
      return [] as AppOption[]
    })
  return loaded
}

watch(sessionVersion, () => {
  loaded = null
  appOptions.value = []
})

/**
 * 当前应用选择。onChange 在选项到位、appId 落到一个可见应用后调用一次，之后每次切换再调用；
 * 页面在这里取数即可，不必自己区分「首次」与「切换」。
 */
export function useAppSelect(page: string, onChange?: (appId: number | null) => void) {
  const storageKey = `scaffold.app.${page}`
  let saved: number | null = null
  try {
    const raw = localStorage.getItem(storageKey)
    if (raw) saved = Number(raw)
  } catch {
    // 存储不可用时从第一个应用开始
  }
  const appId = ref<number | null>(saved)
  let settled = false
  const ready = ensureAppOptions().then((rows) => {
    if (!rows.some(a => a.id === appId.value)) appId.value = rows[0]?.id ?? null
    settled = true
    onChange?.(appId.value)
  })
  // flush: sync 保证 ready 里的赋值不会再触发一次 onChange
  watch(appId, (value) => {
    try { if (value) localStorage.setItem(storageKey, String(value)) } catch { /* 忽略 */ }
    if (settled) onChange?.(value)
  }, { flush: 'sync' })
  const options = computed(() => appOptions.value.map(a => ({ label: a.name, value: a.id, hint: a.code })))
  const current = computed(() => appOptions.value.find(a => a.id === appId.value) ?? null)
  return { appId, options, current, ready }
}

/* ── 扩展字段的值：列表展示与表单输入共用 ── */
export const FIELD_TYPE_LABEL: Record<string, string> = { string: '文本', number: '数字', bool: '布尔', date: '日期', enum: '枚举', counter: '数值账户' }

export const enumOptions = (field: AppFieldRow) => (Array.isArray(field.options) ? field.options : [])
export const counterRule = (field: AppFieldRow) => (field.options && !Array.isArray(field.options) ? field.options : {})

/** 列表单元格文本：账户余额取 counters，其余取 profile；布尔显示 是 / 否 */
export function fieldText(field: AppFieldRow, profile: Record<string, unknown> | undefined, counters: Record<string, number> | undefined) {
  if (field.type === 'counter') return String(counters?.[field.key] ?? 0)
  const value = profile?.[field.key]
  if (value === null || value === undefined || value === '') return '—'
  if (field.type === 'bool') return value === true || value === 'true' || value === 1 ? '是' : '否'
  return String(value)
}

/** 表单输入的初始值：bool 为布尔，number 为数字或 null，其余字符串 */
export function fieldInput(field: AppFieldRow, raw: unknown) {
  if (field.type === 'bool') return raw === true || raw === 'true' || raw === 1
  if (field.type === 'number') return raw === null || raw === undefined || raw === '' ? null : Number(raw)
  return raw === null || raw === undefined ? '' : String(raw)
}

/** 表单值 → 提交的 profile 值；空值返回 undefined（不提交） */
export function fieldOutput(field: AppFieldRow, value: unknown) {
  if (field.type === 'bool') return Boolean(value)
  if (value === null || value === undefined || value === '') return undefined
  if (field.type === 'number') return Number(value)
  return String(value)
}
