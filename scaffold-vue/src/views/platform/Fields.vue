<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Field, Select, StatusTag, Switch, Tag, TextInput, type Column } from '@/components/ui'
import { AppContext, DictSelect, DictTag, FormDialog, ListPage, PermissionButton } from '@/components/business'
import { platformApi, type AppFieldRow, type AppFieldType, type AppOption } from '@/api'
import { useListPage } from '@/composables/list'
import { counterRule, enumOptions, FIELD_TYPE_LABEL, useAppSelect } from '@/composables/platform'
import { confirm } from '@/composables/confirm'
import { pushToast, toastError } from '@/composables/toast'
import { pattern, required, validate } from '@/utils/validate'

/* ============================================================
   字段配置：运营给某个应用定义的用户资料字段与数值账户。
   键与类型创建后不可改；数值账户不允许用户自改。
   ============================================================ */
const app = useAppSelect((id) => {
  list.clear()
  dialog.value = false
  if (id) list.search()
})
const list = useListPage<AppFieldRow, { keyword: string }>({
  query: { keyword: '' },
  toParams: query => ({ ...query, appId: app.appId.value }),
  fetch: async (params) => {
    if (!params.appId) return { rows: [], total: 0, pageNum: 1, pageSize: 1 }
    const key = String(params.keyword ?? '').toLowerCase()
    const rows = (await platformApi.fields.list(Number(params.appId))).filter(f => !key || f.key.toLowerCase().includes(key) || f.label.toLowerCase().includes(key))
    return { rows, total: rows.length, pageNum: 1, pageSize: Math.max(1, rows.length) }
  },
  pageSize: 200,
  immediate: false,
})

const columns: Column[] = [
  { key: 'key', title: '字段键', minWidth: 150 },
  { key: 'label', title: '名称', minWidth: 120 },
  { key: 'type', title: '类型', width: 110 },
  { key: 'required', title: '必填', width: 72 },
  { key: 'userEditable', title: '用户可改', width: 90 },
  { key: 'defaultValue', title: '默认值 / 规则', minWidth: 160 },
  { key: 'sort', title: '排序', width: 72, align: 'right' },
  { key: 'status', title: '状态', width: 90 },
  { key: 'actions', title: '操作', width: 96, className: 'col-actions' },
]

const ruleText = (row: AppFieldRow) => {
  if (row.type === 'enum') return enumOptions(row).join(' / ')
  if (row.type === 'counter') {
    const rule = counterRule(row)
    return `${rule.allowNegative ? '允许为负' : '不可为负'}${rule.max !== null && rule.max !== undefined ? ` · 上限 ${rule.max}` : ''}`
  }
  return row.defaultValue ?? ''
}

/* ── 新增 / 编辑 ── */
interface FieldForm {
  id?: number
  key: string
  label: string
  type: AppFieldType
  required: boolean
  userEditable: boolean
  defaultValue: string
  optionsText: string
  allowNegative: boolean
  max: number | null
  sort: number | null
  status: number
  remark: string
}
const emptyForm = (): FieldForm => ({ key: '', label: '', type: 'string', required: false, userEditable: false, defaultValue: '', optionsText: '', allowNegative: false, max: null, sort: (list.rows.value.length + 1) * 10, status: 1, remark: '' })
const dialog = ref(false)
const formApp = ref<AppOption | null>(null)
const saving = ref(false)
const form = reactive<FieldForm>(emptyForm())
const errors = ref<Partial<Record<keyof FieldForm, string>>>({})
const isEdit = computed(() => Boolean(form.id))
const parsedOptions = computed(() => form.optionsText.split(/[,，\n]/).map(s => s.trim()).filter(Boolean))
const enumChoices = computed(() => parsedOptions.value.map(v => ({ label: v, value: v })))
// 数值账户不允许用户自改
watch(() => form.type, (type) => { if (type === 'counter') form.userEditable = false })

function openCreate() {
  if (!app.current.value) return
  formApp.value = app.current.value
  Object.assign(form, emptyForm(), { id: undefined })
  errors.value = {}
  dialog.value = true
}

function openEdit(row: AppFieldRow) {
  if (row.appId !== app.appId.value) return
  formApp.value = app.current.value
  const rule = counterRule(row)
  Object.assign(form, {
    id: row.id, key: row.key, label: row.label, type: row.type,
    required: row.required === 1, userEditable: row.userEditable === 1,
    defaultValue: row.defaultValue ?? '', optionsText: enumOptions(row).join(', '),
    allowNegative: Boolean(rule.allowNegative), max: rule.max ?? null,
    sort: row.sort, status: row.status, remark: row.remark ?? '',
  })
  errors.value = {}
  dialog.value = true
}

async function submit() {
  if (!formApp.value || formApp.value.id !== app.appId.value) return
  errors.value = validate(form, {
    key: [required('请输入字段键'), pattern(/^[A-Za-z][A-Za-z0-9_]{0,63}$/, '字母开头，1–64 位字母、数字、下划线')],
    label: [required('请输入字段名称')],
    type: [required('请选择类型')],
    optionsText: form.type === 'enum' ? [() => (parsedOptions.value.length ? undefined : '枚举至少需要一个可选值')] : [],
    defaultValue: form.type === 'enum' && form.defaultValue ? [() => (parsedOptions.value.includes(form.defaultValue) ? undefined : '默认值必须是可选值之一')] : [],
    max: form.type === 'counter' ? [v => (v === null || v === undefined || Number(v) > 0 ? undefined : '上限必须大于 0')] : [],
  })
  if (Object.keys(errors.value).length) return
  saving.value = true
  const options = form.type === 'enum' ? parsedOptions.value : form.type === 'counter' ? { allowNegative: form.allowNegative, max: form.max === null ? null : Number(form.max) } : null
  const payload = {
    label: form.label.trim(), required: form.required ? 1 : 0, userEditable: form.type === 'counter' ? 0 : form.userEditable ? 1 : 0,
    defaultValue: form.defaultValue === '' ? null : form.defaultValue, options, sort: form.sort ?? 0, status: form.status, remark: form.remark,
  }
  try {
    if (form.id) {
      await platformApi.fields.update(form.id, payload)
      pushToast('字段已更新')
    } else {
      await platformApi.fields.create({ ...payload, appId: formApp.value.id, key: form.key.trim(), type: form.type })
      pushToast('字段已新增')
    }
    dialog.value = false
    await list.load()
  } catch (error) {
    toastError(error, '字段保存失败')
  } finally {
    saving.value = false
  }
}

async function remove(row: AppFieldRow) {
  const ok = await confirm({
    title: '删除字段',
    message: row.type === 'counter'
      ? `确认删除数值账户「${row.label}」吗？用户的余额与流水保留，但开放接口不再接受该账户。`
      : `确认删除字段「${row.label}」吗？用户资料里的旧值保留，表单与接口不再使用该字段。`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await platformApi.fields.remove(row.id)
    pushToast('字段已删除')
    await list.load()
  } catch (error) {
    toastError(error, '字段删除失败')
  }
}
</script>

<template>
  <ListPage title="字段配置" desc="为应用定义用户资料字段与数值账户；注册、资料与后台用户表单按此渲染。" table-title="字段列表" :list="list" :columns="columns" :min-width="960">
    <template #actions>
      <PermissionButton perm="platform:field:add" variant="primary" icon="plus" :disabled="!app.appId.value" @click="openCreate">新增字段</PermissionButton>
    </template>

    <template #before><AppContext /></template>

    <template #search>
      <Field label="关键词" wide>
        <TextInput v-model="list.query.keyword" icon="search" placeholder="字段键 / 名称" clearable @enter="list.search()" />
      </Field>
    </template>

    <template #cell-key="{ row }"><span class="code-chip">{{ row.key }}</span></template>
    <template #cell-label="{ row }">
      <div style="min-width: 0">
        <div class="cell-title ellipsis">{{ row.label }}</div>
        <div v-if="row.remark" class="cell-sub ellipsis">{{ row.remark }}</div>
      </div>
    </template>
    <template #cell-type="{ row }"><DictTag type="app_field_type" :value="row.type" /></template>
    <template #cell-required="{ row }"><Tag :tone="row.required === 1 ? 'info' : 'neutral'">{{ row.required === 1 ? '必填' : '选填' }}</Tag></template>
    <template #cell-userEditable="{ row }"><span :class="row.userEditable === 1 ? 'muted' : 'faint'">{{ row.userEditable === 1 ? '可改' : '仅后台' }}</span></template>
    <template #cell-defaultValue="{ row }"><span class="muted ellipsis" style="display: inline-block; max-width: 240px" :title="ruleText(row)">{{ ruleText(row) || '—' }}</span></template>
    <template #cell-sort="{ row }"><span class="tnum soft">{{ row.sort }}</span></template>
    <template #cell-status="{ row }"><StatusTag :status="row.status" on="启用" off="停用" /></template>
    <template #cell-actions="{ row }">
      <div class="row-actions">
        <PermissionButton perm="platform:field:edit" icon-only icon="edit" label="编辑" @click="openEdit(row)" />
        <PermissionButton perm="platform:field:remove" icon-only icon="trash" label="删除" danger @click="remove(row)" />
      </div>
    </template>

    <FormDialog :open="dialog" :title="isEdit ? '编辑字段' : '新增字段'" :saving="saving" @close="dialog = false" @submit="submit">
      <AppContext readonly :bound-app="formApp" />
      <Field label="字段键" required :error="errors.key" :hint="isEdit ? '创建后不可修改' : '接口里的字段名，如 inviteCode'">
        <TextInput v-model="form.key" placeholder="字母开头" :disabled="isEdit" :invalid="!!errors.key" autocomplete="off" />
      </Field>
      <Field label="名称" required :error="errors.label">
        <TextInput v-model="form.label" placeholder="表单与列表显示的中文名" :invalid="!!errors.label" />
      </Field>
      <Field label="类型" required :error="errors.type" :hint="isEdit ? '创建后不可修改' : `${FIELD_TYPE_LABEL[form.type]}${form.type === 'counter' ? '：余额只能通过加减变动，有流水' : ''}`">
        <DictSelect v-model="form.type" type="app_field_type" :disabled="isEdit" :invalid="!!errors.type" />
      </Field>
      <Field label="显示排序">
        <TextInput v-model="form.sort" type="number" />
      </Field>

      <template v-if="form.type === 'enum'">
        <Field label="可选值" required :error="errors.optionsText" hint="逗号或换行分隔，保存为字符串数组" span>
          <TextInput v-model="form.optionsText" multiline :rows="2" placeholder="如：bronze, silver, gold" :invalid="!!errors.optionsText" />
        </Field>
        <Field label="默认值" :error="errors.defaultValue">
          <Select v-model="form.defaultValue" :options="enumChoices" placeholder="无" clearable :invalid="!!errors.defaultValue" />
        </Field>
      </template>
      <template v-else-if="form.type === 'counter'">
        <Field label="允许为负" group hint="关闭时余额不足的扣减会被拒绝">
          <Switch v-model="form.allowNegative" :label="form.allowNegative ? '允许' : '不允许'" />
        </Field>
        <Field label="余额上限" :error="errors.max" hint="留空不限">
          <TextInput v-model="form.max" type="number" placeholder="选填" :invalid="!!errors.max" />
        </Field>
      </template>
      <Field v-else label="默认值" :hint="form.type === 'bool' ? '填 true / false' : form.type === 'date' ? '格式 yyyy-MM-dd' : undefined">
        <TextInput v-model="form.defaultValue" :type="form.type === 'number' ? 'number' : form.type === 'date' ? 'date' : 'text'" placeholder="选填" />
      </Field>

      <Field v-if="form.type !== 'counter'" label="必填" group hint="注册时必须提供（有默认值的除外）">
        <Switch v-model="form.required" :label="form.required ? '必填' : '选填'" />
      </Field>
      <Field label="用户可改" group :hint="form.type === 'counter' ? '数值账户只能由后台或应用服务端加减' : '开启后用户可在「我的信息」里修改'">
        <Switch v-model="form.userEditable" :disabled="form.type === 'counter'" :label="form.userEditable ? '用户可改' : '仅后台与应用服务端'" />
      </Field>
      <Field label="状态" group hint="停用后不出现在表单，已有值保留">
        <Switch :model-value="form.status === 1" :label="form.status === 1 ? '启用' : '停用'" @update:model-value="form.status = $event ? 1 : 0" />
      </Field>
      <Field label="备注" span>
        <TextInput v-model="form.remark" multiline :rows="2" placeholder="字段用途说明" :maxlength="200" />
      </Field>
    </FormDialog>
  </ListPage>
</template>
