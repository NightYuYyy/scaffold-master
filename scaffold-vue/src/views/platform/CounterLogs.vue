<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { Field, Select, Tag, TextInput, type Column } from '@/components/ui'
import { ListPage } from '@/components/business'
import { platformApi, type AppFieldRow, type CounterLogRow } from '@/api'
import { useListPage } from '@/composables/list'
import { useAppSelect } from '@/composables/platform'

/* 账户流水：某个应用全部数值账户的加减记录；refNo 幂等号可用来对账。 */
const route = useRoute()
const app = useAppSelect('counterLogs', async (id) => {
  list.query.appId = id
  list.query.fieldKey = ''
  if (!id) { list.rows.value = []; list.total.value = 0; return }
  try {
    counters.value = (await platformApi.fields.list(id)).filter(f => f.type === 'counter')
  } catch {
    counters.value = []
  }
  counterOptions.value = [{ label: '全部账户', value: '' }, ...counters.value.map(f => ({ label: f.label, value: f.key }))]
  list.search()
})
const list = useListPage<CounterLogRow, { appId: number | null; userId: string; fieldKey: string; keyword: string }>({
  query: { appId: null, userId: typeof route.query.userId === 'string' ? route.query.userId : '', fieldKey: '', keyword: '' },
  fetch: params => platformApi.counterLogs.list(params),
  exporter: params => platformApi.counterLogs.export(params),
  exportName: '账户流水.csv',
  immediate: false,
})

const columns: Column[] = [
  { key: 'username', title: '用户', minWidth: 160 },
  { key: 'fieldKey', title: '账户', width: 120 },
  { key: 'amount', title: '变动', width: 100, align: 'right' },
  { key: 'balanceAfter', title: '变动后余额', width: 120, align: 'right' },
  { key: 'reason', title: '原因', minWidth: 200 },
  { key: 'refNo', title: '幂等号', minWidth: 180 },
  { key: 'operator', title: '操作者', width: 150 },
  { key: 'createTime', title: '时间', width: 150 },
]

const counters = ref<AppFieldRow[]>([])
const counterOptions = ref<Array<{ label: string; value: string }>>([])
const labelOf = (key: string) => counters.value.find(f => f.key === key)?.label ?? key
</script>

<template>
  <ListPage title="账户流水" desc="数值账户的每一笔加减：来自开放接口的业务变动与后台的人工调整。" table-title="流水记录" :list="list" :columns="columns" :min-width="1080">
    <template #search>
      <Field label="应用">
        <Select v-model="app.appId.value" :options="app.options.value" placeholder="选择应用" filterable empty-text="没有可见的应用" />
      </Field>
      <Field label="账户">
        <Select v-model="list.query.fieldKey" :options="counterOptions" />
      </Field>
      <Field label="用户 ID">
        <TextInput v-model="list.query.userId" type="number" placeholder="精确匹配" clearable @enter="list.search()" />
      </Field>
      <Field label="关键词" wide>
        <TextInput v-model="list.query.keyword" icon="search" placeholder="原因 / 幂等号 / 账号" clearable @enter="list.search()" />
      </Field>
    </template>

    <template #cell-username="{ row }">
      <div style="min-width: 0">
        <div class="cell-title ellipsis">{{ row.username || `用户 ${row.userId}` }}</div>
        <div class="cell-sub tnum">ID {{ row.userId }}</div>
      </div>
    </template>
    <template #cell-fieldKey="{ row }"><Tag>{{ labelOf(row.fieldKey) }}</Tag></template>
    <template #cell-amount="{ row }"><span :class="['tnum', Number(row.amount) < 0 ? 'is-danger-text' : 'is-ok-text']">{{ Number(row.amount) > 0 ? '+' : '' }}{{ row.amount }}</span></template>
    <template #cell-balanceAfter="{ row }"><span class="tnum">{{ row.balanceAfter }}</span></template>
    <template #cell-reason="{ row }"><span class="muted">{{ row.reason }}</span></template>
    <template #cell-refNo="{ row }"><span class="mono soft ellipsis" style="display: inline-block; max-width: 220px" :title="row.refNo">{{ row.refNo }}</span></template>
    <template #cell-operator="{ row }"><span class="mono soft">{{ row.operator }}</span></template>
    <template #cell-createTime="{ row }"><span class="tnum soft">{{ row.createTime }}</span></template>
  </ListPage>
</template>
