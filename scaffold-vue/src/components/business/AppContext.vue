<script setup lang="ts">
import { Field, Select, TextInput } from '@/components/ui'
import type { AppOption } from '@/api'
import { useAppSelect } from '@/composables/platform'

defineProps<{ readonly?: boolean; boundApp?: AppOption | null }>()
const app = useAppSelect()
</script>

<template>
  <Field v-if="readonly" label="所属应用" span hint="自动绑定此应用，保存后不可转移到其他应用">
    <TextInput :model-value="boundApp ? `${boundApp.name}（${boundApp.code}）` : '未选择应用'" readonly />
  </Field>
  <section v-else class="app-context m-surface" aria-label="当前应用范围">
    <Field label="当前应用">
      <Select v-model="app.appId.value" :options="app.options.value" placeholder="选择应用" filterable empty-text="没有可见的应用" />
    </Field>
    <p class="app-context-hint">
      {{ app.current.value ? '仅查看当前应用的数据；新增内容自动归属此应用。切换页面时保持选择。' : '暂无可管理的应用，请先创建应用或联系管理员分配。' }}
    </p>
  </section>
</template>
