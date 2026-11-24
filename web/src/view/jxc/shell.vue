<template>
  <div class="jxc-app flex h-screen overflow-hidden bg-main text-base-text">
    <!-- 左侧导航：深色墨蓝导轨，按「今天要干什么」分组，不按数据表平铺 -->
    <aside class="flex w-[212px] shrink-0 flex-col bg-[var(--jxc-rail-bg)]">
      <div class="flex h-14 shrink-0 items-center gap-2.5 px-5">
        <div class="grid h-7 w-7 place-items-center rounded-md bg-primary text-[13px] font-semibold leading-none text-white">
          进
        </div>
        <div class="min-w-0">
          <div class="truncate text-[13.5px] font-semibold tracking-wide text-white">进销存</div>
          <div class="text-[9.5px] font-medium uppercase tracking-[0.18em] text-[var(--jxc-rail-muted)]">JXC Workspace</div>
        </div>
      </div>

      <nav class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        <div v-for="group in NAV_GROUPS" :key="group.label" class="mb-5 last:mb-0">
          <div class="mb-1.5 px-2.5 text-[10px] font-medium uppercase tracking-[0.14em] text-[var(--jxc-rail-muted)]">
            {{ group.label }}
          </div>
          <router-link
            v-for="item in group.items"
            :key="item.key"
            :to="item.path"
            class="group relative flex h-9 items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors"
            :class="isActive(item.path) ? 'bg-white/10 font-medium text-white' : 'text-[var(--jxc-rail-text)] hover:bg-white/5 hover:text-white/90'"
          >
            <span
              v-if="isActive(item.path)"
              class="absolute left-0 top-1/2 h-4 w-[2px] -translate-y-1/2 rounded-r bg-primary"
              aria-hidden="true"
            ></span>
            <svg-icon
              :icon="item.icon"
              class="h-[17px] w-[17px] shrink-0 transition-colors"
              :class="isActive(item.path) ? 'text-primary-300' : 'text-white/35 group-hover:text-white/60'"
            />
            <span class="truncate">{{ item.label }}</span>
          </router-link>
        </div>
      </nav>

      <div class="shrink-0 border-t border-white/10 px-3 py-3">
        <!-- 用户区：点击展开操作面板（退出登录） -->
        <el-popover
          placement="top-start"
          trigger="click"
          :width="176"
          popper-class="jxc-user-popover"
        >
          <template #reference>
            <button
              type="button"
              class="flex w-full items-center gap-2.5 rounded-md px-1.5 py-1.5 text-left transition-colors hover:bg-white/10"
              title="账户操作"
            >
              <div class="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-white/10 text-[11px] font-semibold text-white">
                {{ userInitial }}
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-[12px] text-white/85">{{ userName }}</div>
                <div class="truncate text-[10px] text-[var(--jxc-rail-muted)]">{{ authorityName }}</div>
              </div>
              <svg-icon icon="lucide:chevron-up" class="h-3.5 w-3.5 shrink-0 text-white/40" />
            </button>
          </template>

          <button
            type="button"
            class="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-[13px] font-medium text-red-500 transition-colors hover:bg-red-50 dark:hover:bg-red-500/10"
            @click="handleLogout"
          >
            <svg-icon icon="lucide:log-out" class="h-4 w-4 shrink-0" />
            <span>退出登录</span>
          </button>
        </el-popover>
        <button
          v-if="isAdmin"
          type="button"
          class="mt-1 flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-[11px] text-white/40 transition-colors hover:bg-white/5 hover:text-white/75"
          @click="router.push('/layout/dashboard')"
        >
          <svg-icon icon="lucide:arrow-left" class="h-3.5 w-3.5" />
          <span>返回管理后台</span>
        </button>
      </div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col">
      <!-- 顶栏只放全局上下文：面包屑、搜索/命令、对账状态、日期 -->
      <header class="flex h-14 shrink-0 items-center gap-3 border-b border-border bg-container px-6">
        <div class="flex min-w-0 items-center gap-2 text-[12px] text-muted-foreground">
          <span class="font-medium text-base-text">进销存</span>
          <span class="text-border">/</span>
          <span class="truncate">{{ currentTitle }}</span>
        </div>

        <div class="ml-auto flex items-center gap-2.5">
          <button
            type="button"
            class="hidden h-8 items-center gap-2 rounded-md border border-border bg-container px-2.5 text-[12px] text-muted-foreground transition-colors hover:bg-muted hover:text-base-text md:flex"
            @click="paletteOpen = true"
          >
            <svg-icon icon="lucide:search" class="h-3.5 w-3.5" />
            <span>搜索或跳转</span>
            <kbd class="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">{{ commandKey }}</kbd>
          </button>

          <button
            type="button"
            class="flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-[12px] font-medium transition-colors"
            :class="recon.className"
            :title="reconDetail"
            @click="goReconcile"
          >
            <svg-icon :icon="recon.icon" class="h-3.5 w-3.5" :class="{ 'animate-spin': recon.spin }" />
            <span>{{ recon.label }}</span>
          </button>

          <span class="hidden h-4 w-px bg-border lg:block" aria-hidden="true"></span>
          <span class="hidden text-[12px] tabular-nums text-muted-foreground lg:block">{{ dateLabel }}</span>
        </div>
      </header>

      <main class="min-h-0 flex-1 overflow-hidden bg-main px-6 py-5">
        <router-view />
      </main>

      <!-- 备案信息：JXC 使用独立外壳，不经 GVA 的 BottomInfo，故在此单独展示 -->
      <footer class="shrink-0 border-t border-border bg-container px-6 py-1.5">
        <SiteBeian />
      </footer>
    </div>

    <CommandPalette v-model="paletteOpen" :items="paletteItems" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CommandPalette from '@/components/jxc/CommandPalette.vue'
import SiteBeian from '@/components/siteBeian/index.vue'
import { useUserStore } from '@/pinia/modules/user'
import { isTypingTarget } from '@/view/jxc/shared/keyboard'
import { ledgerReconcileLast } from '@/api/jxc'
import { formatDateTime } from '@/view/jxc/shared/format'
import './theme.css'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const NAV_GROUPS = [
  {
    label: '经营',
    items: [
      { key: 'workspace', path: '/jxc/workspace', label: '工作台', icon: 'lucide:layout-dashboard', hint: '1', desc: '今日经营概览与待办', keywords: '首页 看板 概览' },
      { key: 'sale', path: '/jxc/sale', label: '开单', icon: 'lucide:shopping-cart', hint: '2', desc: '扫码或搜索商品，直接出库', keywords: '销售 收银 出库' },
      { key: 'purchase', path: '/jxc/purchase', label: '补货', icon: 'lucide:package-plus', hint: '3', desc: '创建采购单，审核后入库', keywords: '采购 进货 供应商' }
    ]
  },
  {
    label: '单据与库存',
    items: [
      { key: 'documents', path: '/jxc/documents', label: '单据', icon: 'lucide:files', hint: '4', desc: '采购、销售、退货与红冲记录', keywords: '订单 审核 退货 红冲' },
      { key: 'inventory', path: '/jxc/inventory', label: '库存', icon: 'lucide:warehouse', hint: '5', desc: '结存、流水、对账与盘点', keywords: '台账 流水 盘点 对账' }
    ]
  },
  {
    label: '分析与资料',
    items: [
      { key: 'report', path: '/jxc/report', label: '报表', icon: 'lucide:bar-chart-3', hint: '6', desc: '月度经营汇总与期间结账', keywords: '月报 毛利 结账' },
      { key: 'master', path: '/jxc/master', label: '基础资料', icon: 'lucide:database', hint: '7', desc: '商品、规格、供应商与仓库', keywords: '商品 规格 仓库 客户 供应商' }
    ]
  }
]

const NAV_FLAT = NAV_GROUPS.flatMap((group) => group.items)
const currentTitle = computed(() => NAV_FLAT.find((item) => isActive(item.path))?.label ?? '工作台')
const isActive = (path) => route.path.startsWith(path)

const paletteItems = computed(() => {
  // 打开面板时生成一次 nonce：已在库存页时也能让 query 变化，触发一次新的对账。
  const run = paletteOpen.value ? Date.now() : 0
  return [
    ...NAV_FLAT.map((item) => ({ ...item, to: item.path, shortcut: 'Alt ' + item.hint })),
    { key: 'quick-sale', path: '/jxc/sale', label: '新建销售单', icon: 'lucide:scan-line', desc: '进入开单台，搜索结果直接加入明细', to: '/jxc/sale', keywords: '快速 开单 扫码' },
    { key: 'quick-reconcile', path: '/jxc/inventory?reconcile=1', label: '一键对账', icon: 'lucide:shield-check', desc: '校验余额与账本累加是否一致', to: `/jxc/inventory?reconcile=1&run=${run}`, keywords: '核对 账实 差异' },
    { key: 'quick-check', path: '/jxc/inventory?tab=check', label: '发起盘点', icon: 'lucide:clipboard-check', desc: '录入实盘数，差异自动记账', to: '/jxc/inventory?tab=check', keywords: '盘点 实盘 差异' }
  ]
})

const userName = computed(() => userStore.userInfo.nickName || userStore.userInfo.userName || '管理员')
const authorityName = computed(() => userStore.userInfo.authority?.authorityName || '工作台用户')
const isAdmin = computed(() => Number(userStore.userInfo.authority?.authorityId) === 888)
const userInitial = computed(() => (userName.value || '管').slice(0, 1).toUpperCase())

// 退出登录：复用 user store 的 LoginOut（拉黑 token、清缓存、跳登录页并 reload）
const handleLogout = () => {
  userStore.LoginOut()
}

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || '')
const commandKey = isMac ? '⌘K' : 'Ctrl K'

const dateLabel = computed(() => {
  const d = new Date()
  const week = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][d.getDay()]
  return `${d.getMonth() + 1}月${d.getDate()}日 ${week}`
})

// ---------- 对账状态常驻：账不平是大事，不该藏在二级页面 ----------
// 首屏只读 /reconcile/last（不触发全仓扫描）；用户点顶栏或库存页按钮时才执行 /reconcile。
const reconState = ref('checking')
const reconDetail = ref('正在读取最近一次对账结果')
let reconTimer = null
const RECON_META = {
  checking: { label: '读取对账', icon: 'lucide:loader-circle', className: 'border-border bg-container text-muted-foreground', spin: true },
  none: { label: '尚未对账', icon: 'lucide:shield', className: 'border-border bg-container text-muted-foreground', spin: false },
  denied: { label: '无对账权限', icon: 'lucide:shield-off', className: 'border-border bg-container text-muted-foreground', spin: false },
  stale: { label: '对账已过期', icon: 'lucide:clock', className: 'border-warning-200 bg-warning-50 text-warning-700', spin: false },
  ok: { label: '账实相符', icon: 'lucide:circle-check', className: 'border-success-200 bg-success-50 text-success-700', spin: false },
  bad: { label: '对账异常', icon: 'lucide:alert-triangle', className: 'border-error-200 bg-error-50 text-error-700', spin: false },
  error: { label: '对账不可用', icon: 'lucide:cloud-off', className: 'border-border bg-container text-muted-foreground', spin: false }
}
const recon = computed(() => RECON_META[reconState.value] || RECON_META.checking)

const setRecon = (state, detail) => {
  reconState.value = state
  reconDetail.value = detail || ''
}

const formatScope = (scope) => {
  if (!scope || scope === 'all') return '全部仓库'
  if (scope.startsWith('warehouse:')) return `仓库 #${scope.slice('warehouse:'.length)}`
  return scope
}

const checkReconcile = async (silent = false) => {
  if (!silent) setRecon('checking', '正在读取最近一次对账结果')
  try {
    const res = await ledgerReconcileLast({})
    if (res.code === 7) return setRecon('denied', '当前角色无对账权限')
    if (res.code !== 0) return setRecon('error', res.msg || '对账接口不可用')
    const data = res.data || {}
    if (!data.checkedAt) {
      return setRecon('none', '还没有对账记录；点击执行一次检查')
    }
    const meta = [
      `范围 ${formatScope(data.scope)}`,
      `检查于 ${formatDateTime(data.checkedAt)}`,
      `耗时 ${Number(data.durationMs) || 0}ms`,
      `${Number(data.checkedSkus) || 0} 个 SKU`
    ].join(' · ')
    if (data.stale) return setRecon('stale', `${meta} · 超过 60 秒，点击重新检查`)
    if (data.ok) return setRecon('ok', meta)
    setRecon('bad', `${meta} · 发现 ${Number(data.count) || 0} 处不一致，点击查看`)
  } catch {
    setRecon('error', '网络异常，暂时无法读取对账结果')
  }
}

// 手动对账完成后库存页会广播；重新读 /last 即可拿到 checkedAt/耗时/SKU 数。
const onReconEvent = () => {
  checkReconcile()
}

// 顶栏对账：不在库存页就跳过去，已经停在库存页则广播让页面直接执行 /reconcile。
const goReconcile = () => {
  if (reconState.value === 'denied') return
  if (route.path === '/jxc/inventory') {
    window.dispatchEvent(new CustomEvent('jxc:run-reconcile'))
    return
  }
  router.push('/jxc/inventory?reconcile=1')
}

// ---------- 全局键盘 ----------
const paletteOpen = ref(false)
watch(() => route.path, () => {
  paletteOpen.value = false
})

const onKeydown = (event) => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    paletteOpen.value = !paletteOpen.value
    return
  }
  if (event.altKey && !event.ctrlKey && !event.metaKey && /^[1-7]$/.test(event.key) && !isTypingTarget(event.target)) {
    event.preventDefault()
    const target = NAV_FLAT[Number(event.key) - 1]
    if (target) router.push(target.path)
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('jxc:reconcile', onReconEvent)
  checkReconcile()
  // 后端结果 TTL 60s；静默轮询一次，避免页面长时间停留时还挂着过期的“账实相符”。
  reconTimer = setInterval(() => checkReconcile(true), 60_000)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('jxc:reconcile', onReconEvent)
  if (reconTimer) clearInterval(reconTimer)
})
</script>
