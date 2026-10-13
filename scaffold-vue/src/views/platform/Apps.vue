<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import Icon from '@/components/Icon.vue'
import { Button, DataTable, Drawer, Field, IconButton, Modal, StatusTag, Switch, Tag, TextInput, type Column } from '@/components/ui'
import { DictSelect, FormDialog, ListPage, PermissionButton } from '@/components/business'
import { platformApi, type AppKeyCreated, type AppKeyRow, type AppRow } from '@/api'
import { useListPage } from '@/composables/list'
import { ensureAppOptions } from '@/composables/platform'
import { confirm } from '@/composables/confirm'
import { pushToast, toastError } from '@/composables/toast'
import { copyText } from '@/utils/clipboard'
import { pattern, required, validate } from '@/utils/validate'

/* ============================================================
   应用管理：接入平台的业务项目。每个应用多把接口密钥，
   密钥明文只在新建时展示一次。
   ============================================================ */
const route = useRoute()
const list = useListPage<AppRow, { keyword: string; status: string }>({
  query: { keyword: '', status: '' },
  fetch: params => platformApi.apps.list(params),
  exporter: params => platformApi.apps.export(params),
  exportName: '应用列表.csv',
})

const columns: Column[] = [
  { key: 'code', title: '编码', minWidth: 150 },
  { key: 'name', title: '名称', minWidth: 160 },
  { key: 'status', title: '状态', width: 96 },
  { key: 'userCount', title: '用户数', width: 96, align: 'right' },
  { key: 'keyCount', title: '密钥数', width: 96, align: 'right' },
  { key: 'createTime', title: '创建时间', width: 150 },
  { key: 'actions', title: '操作', width: 170, className: 'col-actions' },
]

const DEFAULT_EXPIRES = 604800
const expiresText = (seconds: number | null) => {
  const s = Number(seconds ?? 0)
  if (!s) return ''
  return s % 86400 === 0 ? `${s / 86400} 天` : s % 3600 === 0 ? `${s / 3600} 小时` : `${s} 秒`
}

/* ── 新增 / 编辑 ── */
interface AppForm { id?: number; code: string; name: string; status: number; allowRegister: boolean; captchaEnabled: boolean; tokenExpiresSeconds: number | null; remark: string }
const emptyForm = (): AppForm => ({ code: '', name: '', status: 1, allowRegister: true, captchaEnabled: true, tokenExpiresSeconds: DEFAULT_EXPIRES, remark: '' })
const dialog = ref(false)
const saving = ref(false)
const form = reactive<AppForm>(emptyForm())
const errors = ref<Partial<Record<keyof AppForm, string>>>({})
const isEdit = computed(() => Boolean(form.id))

function openCreate() {
  Object.assign(form, emptyForm(), { id: undefined })
  errors.value = {}
  dialog.value = true
}

function openEdit(row: AppRow) {
  Object.assign(form, {
    id: row.id, code: row.code, name: row.name, status: row.status,
    allowRegister: row.allowRegister === 1, captchaEnabled: row.captchaEnabled === 1,
    tokenExpiresSeconds: row.tokenExpiresSeconds, remark: row.remark ?? '',
  })
  errors.value = {}
  dialog.value = true
}

async function submit() {
  errors.value = validate(form, {
    code: [required('请输入应用编码'), pattern(/^[a-z0-9_]{3,64}$/, '3–64 位小写字母、数字、下划线')],
    name: [required('请输入应用名称')],
    tokenExpiresSeconds: [v => (Number(v) > 0 ? undefined : '请输入大于 0 的秒数')],
  })
  if (Object.keys(errors.value).length) return
  saving.value = true
  const payload = {
    name: form.name.trim(), status: form.status,
    allowRegister: form.allowRegister ? 1 : 0, captchaEnabled: form.captchaEnabled ? 1 : 0,
    tokenExpiresSeconds: Number(form.tokenExpiresSeconds), remark: form.remark,
  }
  try {
    if (form.id) {
      await platformApi.apps.update(form.id, payload)
      pushToast('应用已更新')
    } else {
      await platformApi.apps.create({ ...payload, code: form.code.trim() })
      pushToast('应用已新增，可在「密钥」里签发接口密钥')
    }
    dialog.value = false
    ensureAppOptions(true)
    await list.load()
  } catch (error) {
    toastError(error, '应用保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleStatus(row: AppRow) {
  const next = row.status === 1 ? 0 : 1
  if (!next) {
    const ok = await confirm({ title: '停用应用', message: `确认停用「${row.name}」吗？停用后该应用的开放接口全部拒绝，已登录用户的令牌失效。`, confirmText: '停用', danger: true })
    if (!ok) return
  }
  try {
    await platformApi.apps.updateStatus(row.id, next)
    pushToast(`已${next ? '启用' : '停用'}「${row.name}」`)
    ensureAppOptions(true)
    await list.load()
  } catch (error) {
    toastError(error, '状态更新失败')
  }
}

async function remove(row: AppRow) {
  const ok = await confirm({ title: '删除应用', message: `确认删除「${row.name}」（${row.code}）吗？应用下仍有用户时无法删除。`, confirmText: '删除', danger: true })
  if (!ok) return
  try {
    await platformApi.apps.remove(row.id)
    pushToast('应用已删除')
    ensureAppOptions(true)
    await list.load()
  } catch (error) {
    toastError(error, '应用删除失败')
  }
}

/* ── 密钥抽屉 ── */
const keyApp = ref<AppRow | null>(null)
const keys = ref<AppKeyRow[]>([])
const keysLoading = ref(false)
const keyColumns: Column[] = [
  { key: 'name', title: '名称', minWidth: 120 },
  { key: 'apiKey', title: 'API Key', minWidth: 200 },
  { key: 'secretMasked', title: 'Secret', minWidth: 140 },
  { key: 'status', title: '状态', width: 86 },
  { key: 'lastUsedTime', title: '最近使用', width: 150 },
  { key: 'actions', title: '操作', width: 84, className: 'col-actions' },
]
const openBase = `${window.location.origin}/open/v1`

async function openKeys(row: AppRow) {
  keyApp.value = row
  await loadKeys()
}

async function loadKeys() {
  if (!keyApp.value) return
  keysLoading.value = true
  try {
    keys.value = await platformApi.apps.keys.list(keyApp.value.id)
  } catch (error) {
    toastError(error, '密钥加载失败')
  } finally {
    keysLoading.value = false
  }
}

async function copy(text: string, label = '已复制') {
  const ok = await copyText(text)
  pushToast(ok ? label : '浏览器不支持自动复制，请手动选择复制', ok ? 'ok' : 'danger')
}

const keyDialog = ref(false)
const keyName = ref('')
const keyError = ref('')
const keySaving = ref(false)
/** 新建密钥的明文 secret：只在此弹窗展示一次，关闭即丢弃 */
const created = ref<AppKeyCreated | null>(null)

function openKeyCreate() {
  keyName.value = ''
  keyError.value = ''
  keyDialog.value = true
}

async function submitKey() {
  if (!keyApp.value) return
  if (!keyName.value.trim()) { keyError.value = '请输入密钥名称'; return }
  keySaving.value = true
  try {
    created.value = await platformApi.apps.keys.create(keyApp.value.id, keyName.value.trim())
    keyDialog.value = false
    await loadKeys()
    list.load()
  } catch (error) {
    toastError(error, '密钥创建失败')
  } finally {
    keySaving.value = false
  }
}

async function toggleKey(row: AppKeyRow) {
  if (!keyApp.value) return
  const next = row.status === 1 ? 0 : 1
  if (!next) {
    const ok = await confirm({ title: '停用密钥', message: `确认停用密钥「${row.name}」吗？用它签发的用户令牌将全部失效。`, confirmText: '停用', danger: true })
    if (!ok) return
  }
  try {
    await platformApi.apps.keys.updateStatus(keyApp.value.id, row.id, next)
    pushToast(`密钥已${next ? '启用' : '停用'}`)
    await loadKeys()
  } catch (error) {
    toastError(error, '密钥状态更新失败')
  }
}

async function removeKey(row: AppKeyRow) {
  if (!keyApp.value) return
  const ok = await confirm({ title: '删除密钥', message: `确认删除密钥「${row.name}」吗？删除后不可恢复，接入方需换用其他密钥。`, confirmText: '删除', danger: true })
  if (!ok) return
  try {
    await platformApi.apps.keys.remove(keyApp.value.id, row.id)
    pushToast('密钥已删除')
    await loadKeys()
    list.load()
  } catch (error) {
    toastError(error, '密钥删除失败')
  }
}

onMounted(() => { if (route.query.create === '1') openCreate() })
</script>

<template>
  <ListPage title="应用管理" desc="接入平台的业务项目；每个应用签发自己的接口密钥，应用用户按应用隔离。" table-title="应用列表" :list="list" :columns="columns" :min-width="960">
    <template #actions>
      <PermissionButton perm="platform:app:add" variant="primary" icon="plus" @click="openCreate">新增应用</PermissionButton>
    </template>

    <template #search>
      <Field label="关键词" wide>
        <TextInput v-model="list.query.keyword" icon="search" placeholder="编码 / 名称" clearable @enter="list.search()" />
      </Field>
      <Field label="状态">
        <DictSelect v-model="list.query.status" type="sys_normal_disable" all-label="全部状态" />
      </Field>
    </template>

    <template #cell-code="{ row }"><span class="code-chip">{{ row.code }}</span></template>
    <template #cell-name="{ row }">
      <div style="min-width: 0">
        <div class="cell-title ellipsis">{{ row.name }}</div>
        <div class="cell-sub ellipsis">{{ row.allowRegister === 1 ? '开放注册' : '仅后台建用户' }} · {{ row.captchaEnabled === 1 ? '验证码' : '免验证码' }} · 令牌 {{ expiresText(row.tokenExpiresSeconds) }}</div>
      </div>
    </template>
    <template #cell-status="{ row }"><StatusTag :status="row.status" /></template>
    <template #cell-userCount="{ row }"><span class="tnum">{{ row.userCount }}</span></template>
    <template #cell-keyCount="{ row }"><span class="tnum">{{ row.keyCount }}</span></template>
    <template #cell-createTime="{ row }"><span class="tnum soft">{{ row.createTime }}</span></template>
    <template #cell-actions="{ row }">
      <div class="row-actions">
        <PermissionButton perm="platform:appkey:list" icon-only icon="key" label="密钥" @click="openKeys(row)" />
        <PermissionButton perm="platform:app:edit" icon-only icon="edit" label="编辑" @click="openEdit(row)" />
        <PermissionButton perm="platform:app:edit" icon-only :icon="row.status === 1 ? 'ban' : 'power'" :label="row.status === 1 ? '停用' : '启用'" @click="toggleStatus(row)" />
        <PermissionButton perm="platform:app:remove" icon-only icon="trash" label="删除" danger @click="remove(row)" />
      </div>
    </template>

    <FormDialog :open="dialog" :title="isEdit ? '编辑应用' : '新增应用'" :saving="saving" @close="dialog = false" @submit="submit">
      <Field label="应用编码" required :error="errors.code" :hint="isEdit ? '编码创建后不可修改' : '3–64 位小写字母、数字、下划线；用作令牌受众与 API Key 前缀'">
        <TextInput v-model="form.code" placeholder="如：demo" :disabled="isEdit" :invalid="!!errors.code" autocomplete="off" />
      </Field>
      <Field label="应用名称" required :error="errors.name">
        <TextInput v-model="form.name" placeholder="如：示例应用" :invalid="!!errors.name" />
      </Field>
      <Field label="令牌有效期（秒）" required :error="errors.tokenExpiresSeconds" :hint="`用户登录令牌的有效期，默认 ${DEFAULT_EXPIRES}（7 天）${form.tokenExpiresSeconds ? ` · 当前 ${expiresText(form.tokenExpiresSeconds)}` : ''}`">
        <TextInput v-model="form.tokenExpiresSeconds" type="number" :invalid="!!errors.tokenExpiresSeconds" />
      </Field>
      <Field label="应用状态" group>
        <Switch :model-value="form.status === 1" :label="form.status === 1 ? '正常，开放接口可用' : '停用，拒绝全部开放接口'" @update:model-value="form.status = $event ? 1 : 0" />
      </Field>
      <Field label="开放注册" group hint="关闭后只能由后台或应用服务端创建用户">
        <Switch v-model="form.allowRegister" :label="form.allowRegister ? '允许用户自助注册' : '不允许自助注册'" />
      </Field>
      <Field label="注册登录验证码" group hint="开启后注册、登录需先取验证码">
        <Switch v-model="form.captchaEnabled" :label="form.captchaEnabled ? '开启' : '关闭'" />
      </Field>
      <Field label="备注" span>
        <TextInput v-model="form.remark" multiline :rows="2" placeholder="应用用途说明" :maxlength="200" />
      </Field>
    </FormDialog>

    <Drawer :open="Boolean(keyApp)" origin-from="tr" :title="`密钥 · ${keyApp?.name ?? ''}`" :sub="keyApp?.code" :width="680" @close="keyApp = null">
      <div class="drawer-stack">
        <div class="drawer-toolbar">
          <span class="soft" style="font-size: 12px">共 {{ keys.length }} 把；Secret 只在新建时展示一次</span>
          <PermissionButton perm="platform:appkey:add" variant="primary" size="sm" icon="plus" @click="openKeyCreate">新增密钥</PermissionButton>
        </div>
        <div class="m-surface panel" style="overflow: hidden">
          <DataTable :columns="keyColumns" :data="keys" :loading="keysLoading" :min-width="620" empty-text="还没有密钥，新增一把后接入方才能调用开放接口">
            <template #cell-name="{ row }"><span class="cell-title">{{ row.name }}</span></template>
            <template #cell-apiKey="{ row }">
              <span class="copy-row">
                <span class="code-chip ellipsis">{{ row.apiKey }}</span>
                <IconButton name="copy" label="复制 API Key" small @click="copy(row.apiKey, 'API Key 已复制')" />
              </span>
            </template>
            <template #cell-secretMasked="{ row }"><span class="mono soft">{{ row.secretMasked }}</span></template>
            <template #cell-status="{ row }"><StatusTag :status="row.status" /></template>
            <template #cell-lastUsedTime="{ row }"><span class="tnum soft">{{ row.lastUsedTime || '未使用' }}</span></template>
            <template #cell-actions="{ row }">
              <div class="row-actions">
                <PermissionButton perm="platform:appkey:remove" icon-only :icon="row.status === 1 ? 'ban' : 'power'" :label="row.status === 1 ? '停用' : '启用'" @click="toggleKey(row)" />
                <PermissionButton perm="platform:appkey:remove" icon-only icon="trash" label="删除" danger @click="removeKey(row)" />
              </div>
            </template>
          </DataTable>
        </div>

        <div class="section-label" style="margin: 20px 0 10px">接入说明</div>
        <div class="kv-grid">
          <div class="kv span-2"><div class="kv-label">开放接口地址</div><div class="kv-value copy-row"><span class="mono">{{ openBase }}</span><IconButton name="copy" label="复制地址" small @click="copy(openBase, '地址已复制')" /></div></div>
          <div class="kv"><div class="kv-label">应用标识（前端可带）</div><div class="kv-value mono">X-App-Key: &lt;apiKey&gt;</div></div>
          <div class="kv"><div class="kv-label">应用密钥（仅服务端）</div><div class="kv-value mono">X-App-Secret: &lt;apiSecret&gt;</div></div>
          <div class="kv span-2"><div class="kv-label">用户令牌</div><div class="kv-value mono">Authorization: Bearer &lt;token&gt;</div></div>
          <div class="kv span-2"><div class="kv-label">完整契约</div><div class="kv-value">接口清单、字段与错误码见仓库文档 <span class="code-chip">docs/platform-api.md</span></div></div>
        </div>
      </div>
    </Drawer>

    <FormDialog :open="keyDialog" title="新增密钥" :sub="keyApp?.name" :width="440" :saving="keySaving" submit-text="签发" @close="keyDialog = false" @submit="submitKey">
      <Field label="密钥名称" required :error="keyError" hint="用于区分不同环境或接入方，如「生产」「测试」" span>
        <TextInput v-model="keyName" placeholder="如：生产环境" :invalid="!!keyError" autocomplete="off" />
      </Field>
    </FormDialog>

    <Modal :open="Boolean(created)" title="密钥已签发" sub="请立即保存 Secret，关闭后无法再次查看" :width="520" @close="created = null">
      <template v-if="created">
        <p class="secret-warn"><Icon name="alert" :size="15" />Secret 明文只展示这一次，平台不保存明文；丢失只能删除后重新签发。</p>
        <div class="kv-grid">
          <div class="kv span-2"><div class="kv-label">API Key</div><div class="kv-value copy-row"><span class="mono">{{ created.apiKey }}</span><IconButton name="copy" label="复制 API Key" small @click="copy(created.apiKey, 'API Key 已复制')" /></div></div>
          <div class="kv span-2"><div class="kv-label">API Secret</div><div class="kv-value copy-row"><span class="mono">{{ created.apiSecret }}</span><IconButton name="copy" label="复制 Secret" small @click="copy(created.apiSecret, 'Secret 已复制')" /></div></div>
        </div>
      </template>
      <template #footer>
        <Button variant="primary" @click="created = null">我已保存</Button>
      </template>
    </Modal>
  </ListPage>
</template>
