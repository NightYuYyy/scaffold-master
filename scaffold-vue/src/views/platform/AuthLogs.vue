<script setup lang="ts">
import { Field, Select, Tag, TextInput, type Column } from '@/components/ui'
import { AppContext, ListPage } from '@/components/business'
import { platformApi, type AuthLogRow } from '@/api'
import { useListPage } from '@/composables/list'
import { useAppSelect } from '@/composables/platform'

/* 认证日志：应用用户的注册、登录、登出、刷新及失败记录（含来源 IP）。 */
const app = useAppSelect((id) => {
  list.clear()
  if (id) list.search()
})
const list = useListPage<AuthLogRow, { event: string; status: string; keyword: string }>({
  query: { event: '', status: '', keyword: '' },
  toParams: query => ({ ...query, appId: app.appId.value }),
  fetch: async params => params.appId ? platformApi.authLogs.list(params) : { rows: [], total: 0, pageNum: 1, pageSize: 10 },
  exporter: params => {
    if (!params.appId) throw new Error('请先选择应用')
    return platformApi.authLogs.export(params)
  },
  exportName: '认证日志.csv',
  immediate: false,
})

const columns: Column[] = [
  { key: 'username', title: '账号', minWidth: 160 },
  { key: 'event', title: '事件', width: 110 },
  { key: 'status', title: '结果', width: 90 },
  { key: 'msg', title: '说明', minWidth: 200 },
  { key: 'ip', title: '来源 IP', width: 150 },
  { key: 'eventTime', title: '时间', width: 150 },
]

const EVENT_LABEL: Record<string, string> = { register: '注册', login: '登录', logout: '登出', refresh: '刷新令牌', login_failed: '登录失败', register_failed: '注册失败' }
const EVENTS = [{ label: '全部事件', value: '' }, ...Object.entries(EVENT_LABEL).map(([value, label]) => ({ label, value }))]
const STATUSES = [{ label: '全部结果', value: '' }, { label: '成功', value: '1' }, { label: '失败', value: '0' }]
</script>

<template>
  <ListPage title="认证日志" desc="应用用户的注册、登录、登出与刷新记录，失败记录可用于发现撞库与异常来源。" table-title="认证记录" :list="list" :columns="columns" :min-width="900">
    <template #before><AppContext /></template>
    <template #search>
      <Field label="事件">
        <Select v-model="list.query.event" :options="EVENTS" />
      </Field>
      <Field label="结果">
        <Select v-model="list.query.status" :options="STATUSES" />
      </Field>
      <Field label="关键词" wide>
        <TextInput v-model="list.query.keyword" icon="search" placeholder="账号 / IP" clearable @enter="list.search()" />
      </Field>
    </template>

    <template #cell-username="{ row }">
      <div style="min-width: 0">
        <div class="cell-title ellipsis">{{ row.username || '—' }}</div>
        <div v-if="row.userId" class="cell-sub tnum">ID {{ row.userId }}</div>
      </div>
    </template>
    <template #cell-event="{ row }"><Tag :tone="/failed/.test(row.event) ? 'danger' : row.event === 'logout' ? 'neutral' : 'info'">{{ EVENT_LABEL[row.event] ?? row.event }}</Tag></template>
    <template #cell-status="{ row }"><Tag :tone="row.status === 1 ? 'ok' : 'danger'" dot>{{ row.status === 1 ? '成功' : '失败' }}</Tag></template>
    <template #cell-msg="{ row }"><span class="muted">{{ row.msg || '—' }}</span></template>
    <template #cell-ip="{ row }"><span class="tnum">{{ row.ip || '—' }}</span></template>
    <template #cell-eventTime="{ row }"><span class="tnum soft">{{ row.eventTime }}</span></template>
  </ListPage>
</template>
