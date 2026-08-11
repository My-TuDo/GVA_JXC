<template>
  <div>
    <!-- AI 智能助手悬浮入口 -->
    <button class="ai-fab" title="AI 智能助手" @click="aiVisible = true">
      <svg-icon local-icon="sparkles-gva" class="h-6 w-6" />
    </button>
    <AiAssistant v-model="aiVisible" initial-mode="analyze" />

    <!-- 历史销售统计 -->
    <el-card shadow="never" class="mb-4">
      <template #header>
        <div class="flex-between">
          <span>历史销售统计</span>
          <div class="history-tools">
            <el-date-picker v-model="historyRange" type="daterange" value-format="YYYY-MM-DD" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" size="small" style="width: 260px" @change="loadHistory" />
            <el-radio-group v-model="historyGranularity" size="small" class="ml-2" @change="loadHistory">
              <el-radio-button value="day">按日</el-radio-button>
              <el-radio-button value="month">按月</el-radio-button>
            </el-radio-group>
          </div>
        </div>
      </template>
      <el-table :data="historyList" border size="small" max-height="300">
        <el-table-column label="日期" width="140" prop="date" />
        <el-table-column label="销售额" align="right">
          <template #default="{ row }"><b class="col-amount">¥ {{ row.sales.toFixed(2) }}</b></template>
        </el-table-column>
        <el-table-column label="订单数" width="100" align="right" prop="orders" />
        <el-table-column label="毛利" align="right">
          <template #default="{ row }">
            <span :style="{ color: row.profit >= 0 ? '#67c23a' : '#f56c6c' }">¥ {{ row.profit.toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="合计" width="220" align="right">
          <template #default>
            <span>销售额 <b class="col-amount">¥ {{ historyTotal.sales.toFixed(2) }}</b> · 毛利 <b :style="{ color: historyTotal.profit >= 0 ? '#67c23a' : '#f56c6c' }">¥ {{ historyTotal.profit.toFixed(2) }}</b></span>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!historyList.length" description="选择日期范围查看历史销售" :image-size="60" />
    </el-card>

    <!-- 概览卡片 -->
    <el-row :gutter="16" class="mb-4">
      <el-col v-for="ov in overview" :key="ov.label" :span="8">
        <el-card shadow="never">
          <div class="ov-label">{{ ov.label }}</div>
          <div class="ov-row">销售额 <b class="ov-sales">¥ {{ ov.sales.toFixed(2) }}</b></div>
          <div class="ov-row">毛利 <b :class="ov.profit >= 0 ? 'ov-profit' : 'ov-loss'">¥ {{ ov.profit.toFixed(2) }}</b></div>
          <div class="ov-row">订单数 <b>{{ ov.orders }}</b> · 退货额 <b class="ov-loss">¥ {{ ov.returnAmt.toFixed(2) }}</b></div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <!-- 趋势 -->
      <el-col :span="14">
        <el-card shadow="never" class="mb-4">
          <template #header>近 {{ trendDays }} 天销售趋势</template>
          <VCharts :option="trendOption" autoresize style="height: 300px; width: 100%" />
        </el-card>
        <!-- 分类占比 -->
        <el-card shadow="never">
          <template #header>近 30 天分类销售占比</template>
          <VCharts :option="categoryOption" autoresize style="height: 280px; width: 100%" />
        </el-card>
      </el-col>

      <!-- 右侧：热销 + 预警 -->
      <el-col :span="10">
        <el-card shadow="never" class="mb-4">
          <template #header>热销商品 TOP{{ topLimit }}</template>
          <VCharts :option="topOption" autoresize style="height: 300px; width: 100%" />
        </el-card>
        <el-card shadow="never">
          <template #header>
            <div class="flex-between">
              <span>库存预警</span>
              <el-tag v-if="stockAlerts.length" type="danger" size="small">{{ stockAlerts.length }} 项</el-tag>
            </div>
          </template>
          <el-table :data="stockAlerts" border size="small" max-height="260">
            <el-table-column label="SKU" prop="skuCode" width="140" />
            <el-table-column label="商品" prop="goodsName" min-width="90" show-overflow-tooltip />
            <el-table-column label="可售" width="70" align="right">
              <template #default="{ row }">
                <span style="color: #f56c6c">{{ row.available }}</span>
              </template>
            </el-table-column>
            <el-table-column label="安全线" width="70" align="right" prop="safeStock" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import VCharts from 'vue-echarts'
// vue-echarts 7 为按需版，必须显式注册渲染器与图表组件
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'

use([CanvasRenderer, LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent])
import {
  getDashboardOverview, getDashboardTrend, getDashboardTop,
  getDashboardStockAlert, getDashboardCategory, getSalesHistory,
} from '@/api/jxc/dashboard'
import AiAssistant from '../ai/AiAssistant.vue'

const overview = ref([])
const stockAlerts = ref([])
const trendDays = 14
const topLimit = 10
const aiVisible = ref(false)

// 历史销售统计
const historyRange = ref([])
const historyGranularity = ref('day')
const historyList = ref([])
const historyTotal = ref({ sales: 0, profit: 0 })

const loadHistory = async () => {
  const [from, to] = historyRange.value || []
  if (!from || !to) {
    historyList.value = []
    historyTotal.value = { sales: 0, profit: 0 }
    return
  }
  const { data } = await getSalesHistory(from, to, historyGranularity.value)
  historyList.value = data || []
  historyTotal.value = (data || []).reduce(
    (acc, it) => ({ sales: acc.sales + it.sales, profit: acc.profit + it.profit }),
    { sales: 0, profit: 0 }
  )
}

const trendOption = ref({})
const topOption = ref({})
const categoryOption = ref({})

const loadAll = async () => {
  const [ov, trend, topData, alertData, catData] = await Promise.all([
    getDashboardOverview(),
    getDashboardTrend(trendDays),
    getDashboardTop(30, topLimit),
    getDashboardStockAlert(),
    getDashboardCategory(30),
  ])
  overview.value = ov.data || []
  stockAlerts.value = alertData.data || []

  const t = trend.data || []
  trendOption.value = {
    tooltip: { trigger: 'axis' },
    legend: { data: ['销售额', '订单数'] },
    grid: { left: 60, right: 40, top: 40, bottom: 30 },
    xAxis: { type: 'category', data: t.map((i) => i.date.slice(5)) },
    yAxis: [
      { type: 'value', name: '销售额' },
      { type: 'value', name: '订单数' },
    ],
    series: [
      { name: '销售额', type: 'line', smooth: true, areaStyle: { opacity: 0.15 }, data: t.map((i) => i.sales) },
      { name: '订单数', type: 'bar', yAxisIndex: 1, barWidth: 12, data: t.map((i) => i.orders) },
    ],
  }

  const topList = topData.data || []
  topOption.value = {
    tooltip: { trigger: 'axis' },
    grid: { left: 90, right: 40, top: 30, bottom: 30 },
    xAxis: { type: 'value' },
    yAxis: { type: 'category', data: topList.map((i) => (i.skuCode || '').slice(0, 14)).reverse() },
    series: [{ type: 'bar', barWidth: 14, itemStyle: { color: '#409eff' }, data: topList.map((i) => i.qty).reverse() }],
  }

  const catList = catData.data || []
  categoryOption.value = {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0 },
    series: [{
      type: 'pie', radius: ['35%', '65%'],
      label: { formatter: '{b}: {d}%' },
      data: catList.map((i) => ({ name: i.name || '未分类', value: i.amount })),
    }],
  }
}

onMounted(loadAll)
</script>

<style scoped>
.mb-4 { margin-bottom: 16px; }
.ov-label { color: #909399; font-size: 13px; margin-bottom: 8px; }
.ov-row { margin: 4px 0; font-size: 13px; color: #606266; }
.ov-sales { color: #303133; font-size: 18px; }
.ov-profit { color: #67c23a; }
.ov-loss { color: #f56c6c; }
.flex-between { display: flex; justify-content: space-between; align-items: center; }
.history-tools { display: flex; align-items: center; }
.ml-2 { margin-left: 8px; }
.col-amount { color: #e6a23c; }
/* AI 悬浮按钮 */
.ai-fab {
  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 99;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border: none;
  border-radius: 50%;
  color: #fff;
  background: var(--el-color-primary);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
  cursor: pointer;
  transition: transform 0.15s;
}
.ai-fab:hover { transform: scale(1.08); }
.ai-fab:active { transform: scale(0.95); }
</style>
