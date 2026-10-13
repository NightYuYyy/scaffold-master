<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Avatar, DataTable, Drawer, Field, Select, StatusTag, Switch, Tag, TextInput, type Column } from '@/components/ui'
import { DictSelect, FormDialog, ListPage, PermissionButton } from '@/components/business'
import { platformApi, type AppFieldRow, type AppUserRow, type CounterLogRow } from '@/api'
import { useListPage } from '@/composables/list'
import { counterRule, enumOptions, fieldInput, fieldOutput, fieldText, useAppSelect } from '@/composables/platform'
import { confirm } from '@/composables/confirm'
import { pushToast, toastError } from '@/composables/toast'
import { revealCascade } from '@/composables/motion'
import { pattern, required, validate } from '@/utils/validate'

/* ============================================================
   应用用户：某个应用的 C 端用户。列与表单随该应用启用的扩展字段变化；
   数值账户只能通过「账户调整」加减，不能直接改值。
   ============================================================ */
const route = useRoute()
const app = useAppSelect('appUsers', async (id) => {
  list.query.appId = id
  if (!id) { fields.value = []; list.rows.value = []; list.total.value = 0; return }
  await loadFields()
  list.search()
})
const fields = ref<AppFieldRow[]>([])
const enabledFields = computed(() => fields.value.filter(f => f.status === 1))
const profileFields = computed(() => enabledFields.value.filter(f => f.type !== 'counter'))
const counterFields = computed(() => enabledFields.value.filter(f => f.type === 'counter'))
const fieldCol = (f: AppFieldRow) => `f_${f.key}`

/* 动态列直接用格式化后的文本，不依赖运行时注册的单元格插槽 */
type Row = AppUserRow & Record<string, unknown>
const decorate = (row: AppUserRow): Row => Object.assign({ ...row }, Object.fromEntries(enabledFields.value.map(f => [fieldCol(f), fieldText(f, row.profile, row.counters)])))

const list = useListPage<Row, { appId: number | null; keyword: string; status: string }>({
  query: { appId: null, keyword: '', status: '' },
  fetch: async params => {
    const page = await platformApi.users.list(params)
    return { ...page, rows: page.rows.map(decorate) }
  },
  exporter: params => platformApi.users.export(params),
  exportName: '应用用户.csv',
  immediate: false,
})

const columns = computed<Column[]>(() => [
  { key: 'username', title: '账号', minWidth: 200 },
  { key: 'status', title: '状态', width: 90 },
  ...enabledFields.value.map(f => ({ key: fieldCol(f), title: f.label, minWidth: 110, align: f.type === 'counter' || f.type === 'number' ? 'right' as const : undefined })),
  { key: 'createTime', title: '注册时间', width: 150 },
  { key: 'lastLoginTime', title: '最近登录', width: 150 },
  { key: 'actions', title: '操作', width: 200, className: 'col-actions' },
])

async function loadFields() {
  if (!app.appId.value) { fields.value = []; return }
  try {
    fields.value = await platformApi.fields.list(app.appId.value)
  } catch (error) {
    fields.value = []
    toastError(error, '字段配置加载失败')
  }
}

onMounted(async () => {
  await app.ready
  if (route.query.create === '1' && app.appId.value) openCreate()
})

/* ── 新增 / 编辑 ── */
interface UserForm { id?: number; username: string; password: string; nickname: string; avatar: string; status: number; profile: Record<string, unknown> }
const emptyForm = (): UserForm => ({ username: '', password: '', nickname: '', avatar: '', status: 1, profile: {} })
const dialog = ref(false)
const saving = ref(false)
const form = reactive<UserForm>(emptyForm())
const errors = ref<Record<string, string>>({})
const isEdit = computed(() => Boolean(form.id))

function fillProfile(source?: Record<string, unknown>) {
  form.profile = Object.fromEntries(profileFields.value.map(f => [f.key, fieldInput(f, source?.[f.key] ?? (source ? null : f.defaultValue))]))
}

function openCreate() {
  Object.assign(form, emptyForm(), { id: undefined })
  fillProfile()
  errors.value = {}
  dialog.value = true
}

function openEdit(row: AppUserRow) {
  Object.assign(form, { id: row.id, username: row.username, password: '', nickname: row.nickname, avatar: row.avatar ?? '', status: row.status })
  fillProfile(row.profile ?? {})
  errors.value = {}
  dialog.value = true
}

function profilePayload() {
  const out: Record<string, unknown> = {}
  profileFields.value.forEach((f) => {
    const value = fieldOutput(f, form.profile[f.key])
    if (value !== undefined) out[f.key] = value
  })
  return out
}

async function submit() {
  const base = isEdit.value
    ? validate(form, { nickname: [required('请输入昵称')] })
    : validate(form, {
      username: [required('请输入账号'), pattern(/^[A-Za-z0-9_.@-]{3,64}$/, '3–64 位字母、数字或 _ . @ -')],
      password: [v => (v && String(v).length < 6 ? '密码至少 6 位' : undefined)],
    })
  const missing: Record<string, string> = {}
  profileFields.value.forEach((f) => {
    const value = fieldOutput(f, form.profile[f.key])
    if (f.required === 1 && f.type !== 'bool' && value === undefined && (f.defaultValue === null || f.defaultValue === undefined || f.defaultValue === '')) missing[f.key] = `请填写${f.label}`
  })
  errors.value = { ...base, ...missing }
  if (Object.keys(errors.value).length) return
  saving.value = true
  try {
    if (form.id) {
      await platformApi.users.update(form.id, { nickname: form.nickname.trim(), avatar: form.avatar.trim() || undefined, profile: profilePayload() })
      pushToast('用户资料已更新')
    } else {
      await platformApi.users.create({
        appId: app.appId.value!, username: form.username.trim(), password: form.password || undefined,
        nickname: form.nickname.trim() || undefined, status: form.status, profile: profilePayload(),
      })
      pushToast('用户已新增')
    }
    dialog.value = false
    await list.load()
  } catch (error) {
    toastError(error, '用户保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleStatus(row: AppUserRow) {
  const next = row.status === 1 ? 0 : 1
  if (!next) {
    const ok = await confirm({ title: '停用用户', message: `确认停用「${row.nickname || row.username}」吗？停用后其登录令牌全部失效。`, confirmText: '停用', danger: true })
    if (!ok) return
  }
  try {
    await platformApi.users.updateStatus(row.id, next)
    pushToast(`已${next ? '启用' : '停用'}「${row.nickname || row.username}」`)
    await list.load()
  } catch (error) {
    toastError(error, '状态更新失败')
  }
}

async function remove(row: AppUserRow) {
  const ok = await confirm({ title: '删除用户', message: `确认删除「${row.nickname || row.username}」吗？账户余额与流水保留，登录令牌失效。`, confirmText: '删除', danger: true })
  if (!ok) return
  try {
    await platformApi.users.remove(row.id)
    pushToast('用户已删除')
    await list.load()
  } catch (error) {
    toastError(error, '用户删除失败')
  }
}

/* ── 重置密码 ── */
const resetTarget = ref<AppUserRow | null>(null)
const resetPassword = ref('')
const resetError = ref('')
const resetting = ref(false)
function openReset(row: AppUserRow) {
  resetTarget.value = row
  resetPassword.value = ''
  resetError.value = ''
}
async function submitReset() {
  if (!resetTarget.value) return
  if (resetPassword.value.length < 6) { resetError.value = '密码至少 6 位'; return }
  resetting.value = true
  try {
    await platformApi.users.resetPassword(resetTarget.value.id, resetPassword.value)
    pushToast(`已重置「${resetTarget.value.nickname || resetTarget.value.username}」的密码，原登录令牌已失效`)
    resetTarget.value = null
  } catch (error) {
    toastError(error, '密码重置失败')
  } finally {
    resetting.value = false
  }
}

/* ── 账户调整 ── */
const adjustTarget = ref<AppUserRow | null>(null)
const adjust = reactive({ key: '', amount: null as number | null, reason: '' })
const adjustErrors = ref<Record<string, string>>({})
const adjusting = ref(false)
const counterOptions = computed(() => counterFields.value.map(f => ({ label: f.label, value: f.key, hint: `余额 ${adjustTarget.value?.counters?.[f.key] ?? 0}` })))
const adjustField = computed(() => counterFields.value.find(f => f.key === adjust.key))
const adjustBalance = computed(() => Number(adjustTarget.value?.counters?.[adjust.key] ?? 0))
const adjustAfter = computed(() => adjustBalance.value + Number(adjust.amount ?? 0))

function openAdjust(row: AppUserRow) {
  adjustTarget.value = row
  Object.assign(adjust, { key: counterFields.value[0]?.key ?? '', amount: null, reason: '' })
  adjustErrors.value = {}
}

async function submitAdjust() {
  if (!adjustTarget.value) return
  adjustErrors.value = validate(adjust, {
    key: [required('请选择账户')],
    amount: [v => (v === null || Number(v) === 0 ? '请输入非 0 的调整数量' : /^-?\d+(\.\d{1,4})?$/.test(String(v)) ? undefined : '最多 4 位小数')],
    reason: [required('请填写调整原因')],
  })
  const rule = adjustField.value ? counterRule(adjustField.value) : {}
  if (!adjustErrors.value.amount && !rule.allowNegative && adjustAfter.value < 0) adjustErrors.value.amount = '该账户不允许为负'
  if (!adjustErrors.value.amount && rule.max !== null && rule.max !== undefined && adjustAfter.value > Number(rule.max)) adjustErrors.value.amount = `超出账户上限 ${rule.max}`
  if (Object.keys(adjustErrors.value).length) return
  adjusting.value = true
  try {
    const result = await platformApi.users.adjustCounter(adjustTarget.value.id, adjust.key, { amount: Number(adjust.amount), reason: adjust.reason.trim() })
    pushToast(`「${adjustField.value?.label ?? adjust.key}」已调整，当前余额 ${result.balance}`)
    adjustTarget.value = null
    await list.load()
    if (detail.value?.id) loadDetail(detail.value.id)
  } catch (error) {
    toastError(error, '账户调整失败')
  } finally {
    adjusting.value = false
  }
}

/* ── 详情抽屉 ── */
const detail = ref<AppUserRow | null>(null)
const detailLogs = ref<CounterLogRow[]>([])
const detailLoading = ref(false)
const detailBody = ref<HTMLElement | null>(null)
const logColumns: Column[] = [
  { key: 'fieldKey', title: '账户', width: 110 },
  { key: 'amount', title: '变动', width: 90, align: 'right' },
  { key: 'balanceAfter', title: '余额', width: 90, align: 'right' },
  { key: 'reason', title: '原因' },
  { key: 'createTime', title: '时间', width: 150 },
]
const fieldLabel = (key: string) => fields.value.find(f => f.key === key)?.label ?? key

async function loadDetail(id: number) {
  detailLoading.value = true
  try {
    const [user, logs] = await Promise.all([
      platformApi.users.get(id),
      platformApi.counterLogs.list({ appId: app.appId.value, userId: id, pageNum: 1, pageSize: 10 }),
    ])
    detail.value = user
    detailLogs.value = logs.rows
    await nextTick()
    revealCascade(detailBody.value)
  } catch (error) {
    toastError(error, '用户详情加载失败')
  } finally {
    detailLoading.value = false
  }
}
function openDetail(row: AppUserRow) {
  detail.value = row
  detailLogs.value = []
  loadDetail(row.id)
}
</script>

<template>
  <ListPage title="应用用户" desc="各应用的终端用户，按应用隔离；资料字段与数值账户随该应用的字段配置变化。" table-title="用户列表" :list="list" :columns="columns" :min-width="960">
    <template #actions>
      <PermissionButton perm="platform:user:add" variant="primary" icon="plus" :disabled="!app.appId.value" @click="openCreate">新增用户</PermissionButton>
    </template>

    <template #search>
      <Field label="应用">
        <Select v-model="app.appId.value" :options="app.options.value" placeholder="选择应用" filterable empty-text="没有可见的应用" />
      </Field>
      <Field label="关键词" wide>
        <TextInput v-model="list.query.keyword" icon="search" placeholder="账号 / 昵称" clearable @enter="list.search()" />
      </Field>
      <Field label="状态">
        <DictSelect v-model="list.query.status" type="sys_normal_disable" all-label="全部状态" />
      </Field>
    </template>

    <template #cell-username="{ row }">
      <div class="cell-main">
        <Avatar :name="row.nickname || row.username" :size="32" />
        <div style="min-width: 0">
          <button type="button" class="link-btn cell-title ellipsis" @click="openDetail(row)">{{ row.nickname || row.username }}</button>
          <div class="cell-sub">@{{ row.username }}</div>
        </div>
      </div>
    </template>
    <template #cell-status="{ row }"><StatusTag :status="row.status" /></template>
    <template #cell-createTime="{ row }"><span class="tnum soft">{{ row.createTime }}</span></template>
    <template #cell-lastLoginTime="{ row }"><span class="tnum soft">{{ row.lastLoginTime || '从未登录' }}</span></template>
    <template #cell-actions="{ row }">
      <div class="row-actions">
        <PermissionButton perm="platform:user:list" icon-only icon="eye" label="详情" @click="openDetail(row)" />
        <PermissionButton perm="platform:user:edit" icon-only icon="edit" label="编辑" @click="openEdit(row)" />
        <PermissionButton v-if="counterFields.length" perm="platform:counter:adjust" icon-only icon="activity" label="账户调整" @click="openAdjust(row)" />
        <PermissionButton perm="platform:user:resetPwd" icon-only icon="key" label="重置密码" @click="openReset(row)" />
        <PermissionButton perm="platform:user:edit" icon-only :icon="row.status === 1 ? 'ban' : 'power'" :label="row.status === 1 ? '停用' : '启用'" @click="toggleStatus(row)" />
        <PermissionButton perm="platform:user:remove" icon-only icon="trash" label="删除" danger @click="remove(row)" />
      </div>
    </template>

    <FormDialog :open="dialog" :title="isEdit ? '编辑用户' : '新增用户'" :sub="app.current.value?.name" :saving="saving" @close="dialog = false" @submit="submit">
      <template v-if="!isEdit">
        <Field label="账号" required :error="errors.username" hint="应用内唯一，3–64 位字母、数字或 _ . @ -">
          <TextInput v-model="form.username" :invalid="!!errors.username" autocomplete="off" />
        </Field>
        <Field label="密码" :error="errors.password" hint="留空则使用参数「应用用户-初始密码」">
          <TextInput v-model="form.password" placeholder="至少 6 位，选填" :invalid="!!errors.password" autocomplete="new-password" />
        </Field>
      </template>
      <Field label="昵称" :required="isEdit" :error="errors.nickname" :hint="isEdit ? undefined : '缺省与账号相同'">
        <TextInput v-model="form.nickname" :invalid="!!errors.nickname" />
      </Field>
      <Field v-if="isEdit" label="头像地址">
        <TextInput v-model="form.avatar" placeholder="图片 URL，选填" />
      </Field>
      <Field v-else label="账号状态" group>
        <Switch :model-value="form.status === 1" :label="form.status === 1 ? '正常' : '停用'" @update:model-value="form.status = $event ? 1 : 0" />
      </Field>
      <template v-for="f in profileFields" :key="f.key">
        <Field v-if="f.type === 'bool'" :label="f.label" group :hint="f.remark">
          <Switch :model-value="Boolean(form.profile[f.key])" :label="form.profile[f.key] ? '是' : '否'" @update:model-value="form.profile[f.key] = $event" />
        </Field>
        <Field v-else :label="f.label" :required="f.required === 1" :error="errors[f.key]" :hint="f.remark || (f.defaultValue ? `默认 ${f.defaultValue}` : undefined)">
          <Select v-if="f.type === 'enum'" :model-value="String(form.profile[f.key] ?? '')" :options="enumOptions(f)" clearable :invalid="!!errors[f.key]" @update:model-value="form.profile[f.key] = $event ?? ''" />
          <TextInput v-else :model-value="form.profile[f.key] as string | number | null" :type="f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : 'text'" :invalid="!!errors[f.key]" @update:model-value="form.profile[f.key] = $event" />
        </Field>
      </template>
      <p v-if="!profileFields.length" class="faint span-2" style="font-size: 12px; margin: 0">该应用没有启用的资料字段，可在「字段配置」中添加。</p>
    </FormDialog>

    <FormDialog :open="Boolean(resetTarget)" title="重置密码" :sub="resetTarget ? `${resetTarget.nickname}（@${resetTarget.username}）` : ''" :width="440" :saving="resetting" submit-text="确认重置" @close="resetTarget = null" @submit="submitReset">
      <Field label="新密码" required :error="resetError" hint="6–64 位；重置后该用户需重新登录" span>
        <TextInput v-model="resetPassword" :invalid="!!resetError" autocomplete="new-password" />
      </Field>
    </FormDialog>

    <FormDialog :open="Boolean(adjustTarget)" title="账户调整" :sub="adjustTarget ? `${adjustTarget.nickname}（@${adjustTarget.username}）` : ''" :width="520" :saving="adjusting" submit-text="确认调整" @close="adjustTarget = null" @submit="submitAdjust">
      <div class="kv-grid span-2">
        <div v-for="f in counterFields" :key="f.key" class="kv">
          <div class="kv-label">{{ f.label }} <span class="mono faint">{{ f.key }}</span></div>
          <div class="kv-value tnum" style="font-size: 18px; font-weight: 680">{{ adjustTarget?.counters?.[f.key] ?? 0 }}</div>
        </div>
      </div>
      <Field label="账户" required :error="adjustErrors.key">
        <Select v-model="adjust.key" :options="counterOptions" :invalid="!!adjustErrors.key" />
      </Field>
      <Field label="调整数量" required :error="adjustErrors.amount" :hint="adjust.amount ? `调整后 ${adjustAfter}` : '正数增加、负数扣减，最多 4 位小数'">
        <TextInput v-model="adjust.amount" type="number" placeholder="如 100 或 -10" :invalid="!!adjustErrors.amount" />
      </Field>
      <Field label="原因" required :error="adjustErrors.reason" span>
        <TextInput v-model="adjust.reason" placeholder="写入流水，如：活动补发" :maxlength="200" :invalid="!!adjustErrors.reason" />
      </Field>
    </FormDialog>

    <Drawer :open="Boolean(detail)" origin-from="tr" :title="detail ? (detail.nickname || detail.username) : ''" :sub="detail ? `@${detail.username} · ${app.current.value?.name ?? ''}` : ''" :width="600" @close="detail = null">
      <div v-if="detail" ref="detailBody" class="drawer-stack">
        <div class="kv-grid">
          <div class="kv"><div class="kv-label">状态</div><div class="kv-value"><StatusTag :status="detail.status" /></div></div>
          <div class="kv"><div class="kv-label">注册 IP</div><div class="kv-value tnum">{{ detail.registerIp || '—' }}</div></div>
          <div class="kv"><div class="kv-label">注册时间</div><div class="kv-value tnum">{{ detail.createTime }}</div></div>
          <div class="kv"><div class="kv-label">最近登录</div><div class="kv-value tnum">{{ detail.lastLoginTime || '从未登录' }}</div></div>
          <div v-for="f in profileFields" :key="f.key" class="kv"><div class="kv-label">{{ f.label }}</div><div class="kv-value">{{ fieldText(f, detail.profile, detail.counters) }}</div></div>
        </div>

        <template v-if="counterFields.length">
          <div class="section-label" style="margin: 20px 0 10px">数值账户</div>
          <div class="kv-grid">
            <div v-for="f in counterFields" :key="f.key" class="kv">
              <div class="kv-label">{{ f.label }} <span class="mono faint">{{ f.key }}</span></div>
              <div class="kv-value tnum" style="font-size: 20px; font-weight: 680">{{ detail.counters?.[f.key] ?? 0 }}</div>
            </div>
          </div>
          <div class="drawer-toolbar" style="margin: 20px 0 10px">
            <span class="section-label">最近流水</span>
            <PermissionButton perm="platform:counter:adjust" variant="secondary" size="sm" icon="activity" @click="openAdjust(detail)">账户调整</PermissionButton>
          </div>
          <div class="m-surface panel" style="overflow: hidden">
            <DataTable :columns="logColumns" :data="detailLogs" :loading="detailLoading" :min-width="520" empty-text="还没有流水">
              <template #cell-fieldKey="{ row }"><Tag>{{ fieldLabel(row.fieldKey) }}</Tag></template>
              <template #cell-amount="{ row }"><span :class="['tnum', Number(row.amount) < 0 ? 'is-danger-text' : 'is-ok-text']">{{ Number(row.amount) > 0 ? '+' : '' }}{{ row.amount }}</span></template>
              <template #cell-balanceAfter="{ row }"><span class="tnum">{{ row.balanceAfter }}</span></template>
              <template #cell-reason="{ row }"><div class="ellipsis" :title="row.reason">{{ row.reason }}<div class="cell-sub mono">{{ row.operator }}</div></div></template>
              <template #cell-createTime="{ row }"><span class="tnum soft">{{ row.createTime }}</span></template>
            </DataTable>
          </div>
        </template>
      </div>
    </Drawer>
  </ListPage>
</template>
