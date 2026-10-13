<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Icon from '@/components/Icon.vue'
import Segmented from '@/components/ui/Segmented.vue'
import Avatar from '@/components/ui/Avatar.vue'
import Tag from '@/components/ui/Tag.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import Metric from '@/components/ui/Metric.vue'
import DeltaPill from '@/components/ui/DeltaPill.vue'
import SectionLabel from '@/components/ui/SectionLabel.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import TrendChart from '@/components/charts/TrendChart.vue'
import BarChart from '@/components/charts/BarChart.vue'
import { portalApi, type DashboardStat, type DashboardSummary } from '@/api'
import { currentUser, navGroups, pathOf } from '@/composables/auth'
import { toastError } from '@/composables/toast'
import { EASE, firstTimeInSession, gsap, motionSkipped, trackGlow } from '@/composables/motion'

/* ============================================================
   工作台 · 主次布局：8 列主卡（注册与登录脉搏）+ 4 列读数侧块，
   下排各应用读数 / 数值账户汇总，底部最近动态与常用入口。
   ============================================================ */
type Range = DashboardSummary['range']

const router = useRouter()
const range = ref<Range>('24h')
const summary = ref<DashboardSummary | null>(null)
const loading = ref(true)
const RANGE_TEXT: Record<Range, string> = { '24h': '近 24 小时', '7d': '近 7 天', '30d': '近 30 天' }
const STAT_ICON: Record<string, string> = { apps: 'grid', users: 'users', operators: 'user', registrations: 'plus', logins: 'lock' }

const greeting = computed(() => {
  const h = new Date().getHours()
  return h < 6 ? '夜深了' : h < 12 ? '上午好' : h < 14 ? '中午好' : h < 18 ? '下午好' : '晚上好'
})
const now = new Date()
const dateStamp = `${now.getFullYear()} 年 ${now.getMonth() + 1} 月 ${now.getDate()} 日 · 星期${'日一二三四五六'[now.getDay()]}`

const statOf = (key: string): DashboardStat | null => summary.value?.stats.find(s => s.key === key) ?? null
const logins = computed(() => statOf('logins'))
const registrations = computed(() => statOf('registrations'))
/* 主卡已经展示登录与注册，其余读数（应用、应用用户、后台账号…）放侧块 */
const sideStats = computed(() => (summary.value?.stats ?? []).filter(s => s.key !== 'logins' && s.key !== 'registrations'))

/* ── 主卡：注册与登录趋势 ── */
const trend = computed(() => summary.value?.trend ?? { labels: [], registrations: [], logins: [] })
const peak = computed(() => Math.max(0, ...trend.value.logins))
const regTotal = computed(() => trend.value.registrations.reduce((a, b) => a + b, 0))

/* ── 各应用读数：表格 + 登录占比柱 ── */
const apps = computed(() => summary.value?.apps ?? [])
const appColumns: Column[] = [
  { key: 'name', title: '应用', minWidth: 160 },
  { key: 'users', title: '用户数', width: 96, align: 'right' },
  { key: 'registrations', title: '新注册', width: 96, align: 'right' },
  { key: 'logins', title: '登录', width: 96, align: 'right' },
]
const counters = computed(() => summary.value?.counters ?? [])
const activities = computed(() => summary.value?.activities ?? [])

/* ── 跨页入口：只在当前账号有对应菜单时出现 ── */
const authLogPath = computed(() => pathOf('platform/authlog/index'))
const appsPath = computed(() => pathOf('platform/app/index'))
const counterLogPath = computed(() => pathOf('platform/counterlog/index'))
const shortcuts = computed(() => navGroups.value
  .filter(g => !g.single)
  .flatMap(g => g.items.map(i => ({ ...i, group: g.label })))
  .slice(0, 6))

async function load() {
  loading.value = true
  try {
    summary.value = await portalApi.dashboard.summary({ range: range.value })
  } catch (error) {
    toastError(error, '工作台数据加载失败')
  } finally {
    loading.value = false
  }
}

/* ── 开场编排：本会话首次进入，卡片从中心弹簧飞出落位 ── */
const grid = ref<HTMLElement | null>(null)
function playEntrance() {
  if (!firstTimeInSession('dashboard') || motionSkipped()) return
  nextTick(() => {
    const box = grid.value?.getBoundingClientRect()
    const cards = grid.value ? [...grid.value.querySelectorAll<HTMLElement>('.bento')] : []
    if (!box || !cards.length) return
    const cx = box.left + box.width / 2
    const cy = box.top + Math.min(box.height / 2, 300)
    cards.forEach((el, i) => {
      const r = el.getBoundingClientRect()
      gsap.from(el, {
        x: cx - (r.left + r.width / 2),
        y: cy - (r.top + r.height / 2),
        scale: 0.82,
        opacity: 0,
        duration: 0.72,
        ease: EASE.settle,
        delay: 0.05 + i * 0.05,
        clearProps: 'transform,opacity',
      })
    })
  })
}

watch(range, load)
onMounted(() => {
  load()
  playEntrance()
})
</script>

<template>
  <div class="page dashboard">
    <header class="page-head reveal">
      <div class="page-head-main">
        <span class="page-date">{{ dateStamp }}</span>
        <h1 class="page-title">{{ greeting }}，{{ currentUser?.nickname }}</h1>
        <p class="page-desc">各应用的用户规模、注册与登录脉搏、数值账户变动。</p>
      </div>
    </header>

    <div ref="grid" class="bento-grid">
      <!-- 主卡：注册与登录脉搏 -->
      <section class="bento bento-hero m-glow" @pointermove="trackGlow">
        <span class="glow-layer" aria-hidden="true" />
        <div class="hero-head">
          <SectionLabel title="注册与登录" />
          <Segmented v-model="range" label="时间范围" :options="[{ label: '24 小时', value: '24h' }, { label: '7 天', value: '7d' }, { label: '30 天', value: '30d' }]" />
        </div>
        <div class="hero-body">
          <div class="hero-figure">
            <Metric label="登录次数" :value="logins?.value" unit="次" size="xl" />
            <div class="hero-delta">
              <DeltaPill v-if="logins" :value="logins.delta" :up="logins.up" :fresh="logins.fresh" label="较上一等长周期" />
              <span class="faint">{{ logins?.sub || '较上一周期' }}</span>
            </div>
            <dl class="hero-readouts">
              <div><dt>新注册</dt><dd class="tnum">{{ registrations?.value ?? regTotal }}</dd></div>
              <div><dt>登录峰值</dt><dd class="tnum">{{ peak }}</dd></div>
              <div><dt>范围</dt><dd>{{ RANGE_TEXT[range] }}</dd></div>
            </dl>
            <DeltaPill v-if="registrations && registrations.delta !== undefined" :value="registrations.delta" :up="registrations.up" :fresh="registrations.fresh" label="新注册较上一周期" />
          </div>
          <div class="hero-chart">
            <TrendChart :values="trend.logins" :secondary="trend.registrations" :labels="trend.labels" :height="212" unit=" 次" primary-label="登录" secondary-label="注册" tone="ink" :dots="false" />
          </div>
        </div>
        <div class="hero-actions">
          <button v-if="authLogPath" type="button" class="pill-btn is-dark" @click="router.push(authLogPath)">
            查看认证日志 <Icon name="arrowRight" :size="14" />
          </button>
          <button type="button" class="pill-btn" :disabled="loading" @click="load">
            <Icon name="refresh" :size="14" /> 刷新
          </button>
        </div>
      </section>

      <!-- 侧块：应用 / 应用用户 / 后台账号 等读数 -->
      <div class="bento-stack">
        <section v-for="s in sideStats" :key="s.key" class="bento bento-side">
          <div class="side-top">
            <span class="side-label">{{ s.label }}</span>
            <span class="side-ico m-groove"><Icon :name="STAT_ICON[s.key] ?? 'activity'" :size="16" /></span>
          </div>
          <Metric :value="s.value" size="l" />
          <div class="side-foot">
            <span class="muted">{{ s.sub || '' }}</span>
            <DeltaPill v-if="s.delta !== undefined" :value="s.delta" :up="s.up" :fresh="s.fresh" label="较上一周期" />
          </div>
        </section>
        <section v-if="!loading && !sideStats.length" class="bento bento-side">
          <EmptyState title="暂无概览读数" icon="activity" />
        </section>
      </div>

      <!-- 各应用读数 -->
      <section class="bento bento-wide">
        <div class="bento-head">
          <SectionLabel title="各应用" />
          <button v-if="appsPath" type="button" class="pill-btn" @click="router.push(appsPath)">应用管理 <Icon name="arrowRight" :size="14" /></button>
        </div>
        <template v-if="apps.length">
          <BarChart v-if="apps.length > 1" :values="apps.map(a => a.logins)" :labels="apps.map(a => a.name)" :height="132" unit=" 次登录" />
          <div class="m-surface panel" style="overflow: hidden; margin-top: 12px">
            <DataTable :columns="appColumns" :data="apps" :loading="loading" :min-width="420">
              <template #cell-name="{ row }">
                <div style="min-width: 0">
                  <div class="cell-title ellipsis">{{ row.name }}</div>
                  <div class="cell-sub mono">{{ row.code }}</div>
                </div>
              </template>
              <template #cell-users="{ row }"><span class="tnum">{{ row.users }}</span></template>
              <template #cell-registrations="{ row }"><span class="tnum">{{ row.registrations }}</span></template>
              <template #cell-logins="{ row }"><span class="tnum">{{ row.logins }}</span></template>
            </DataTable>
          </div>
        </template>
        <EmptyState v-else-if="!loading" title="还没有可见的应用" desc="先在「应用管理」新增应用并签发密钥" icon="grid" />
      </section>

      <!-- 数值账户汇总 -->
      <section class="bento bento-narrow">
        <div class="bento-head">
          <SectionLabel title="数值账户" />
          <button v-if="counterLogPath" type="button" class="pill-btn" @click="router.push(counterLogPath)">流水 <Icon name="arrowRight" :size="14" /></button>
        </div>
        <ul v-if="counters.length" class="counter-list">
          <li v-for="c in counters" :key="`${c.app}-${c.fieldKey}`">
            <span class="counter-main">
              <span class="counter-label ellipsis">{{ c.label }}<small class="mono">{{ c.fieldKey }}</small></span>
              <span class="counter-sub ellipsis">{{ c.app }} · {{ RANGE_TEXT[range] }} {{ c.changes }} 笔变动</span>
            </span>
            <Metric :value="c.total" size="s" />
          </li>
        </ul>
        <EmptyState v-else-if="!loading" title="暂无数值账户" desc="在「字段配置」里新增数值账户类型的字段" icon="activity" />
      </section>

      <!-- 最近动态 -->
      <section class="bento bento-half">
        <div class="bento-head">
          <SectionLabel title="最近动态" />
        </div>
        <div class="activity-list">
          <div v-for="(a, i) in activities" :key="i" class="activity">
            <Avatar :name="String(a.who)" :size="30" />
            <div class="activity-main">
              <div class="ellipsis"><b>{{ a.who }}</b> {{ a.action }} <span class="muted">· {{ a.app }}</span></div>
              <div class="faint mono-time">{{ a.time }}</div>
            </div>
            <Tag :tone="a.tint === 'ok' ? 'ok' : 'danger'" dot>{{ a.tint === 'ok' ? '成功' : '失败' }}</Tag>
          </div>
          <EmptyState v-if="!loading && !activities.length" title="暂无认证动态" icon="clock" />
        </div>
      </section>

      <!-- 常用入口 -->
      <section v-if="shortcuts.length" class="bento bento-half">
        <div class="bento-head">
          <SectionLabel title="常用入口" />
        </div>
        <div class="shortcut-grid">
          <button v-for="s in shortcuts" :key="s.key" type="button" class="quick-link lift m-surface" @click="router.push(s.path)">
            <span class="quick-ico m-groove"><Icon :name="s.icon" :size="17" /></span>
            <span class="quick-text">
              <span class="quick-title">{{ s.label }}</span>
              <span class="quick-sub">{{ s.group }}</span>
            </span>
            <Icon name="arrowRight" :size="15" class="quick-go" />
          </button>
        </div>
      </section>
    </div>
  </div>
</template>
