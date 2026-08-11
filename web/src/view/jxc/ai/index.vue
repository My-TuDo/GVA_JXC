<template>
  <div class="p-4">
    <el-card shadow="never">
      <template #header>
        <div class="flex-between">
          <span>AI 智能助手</span>
          <span class="text-sm text-gray-400">销售业绩分析 · 客户需求商品推荐</span>
        </div>
      </template>
      <div class="mx-auto max-w-3xl">
        <AiAssistant v-model="visible" initial-mode="analyze" />
        <!-- 独立页直接内嵌对话（非悬浮） -->
        <div class="ai-embedded">
          <div class="mb-3 flex gap-2">
            <el-radio-group v-model="mode" size="small">
              <el-radio-button value="analyze">销售业绩分析</el-radio-button>
              <el-radio-button value="recommend">客户需求推荐</el-radio-button>
            </el-radio-group>
          </div>
          <div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-slate-700 dark:bg-slate-800">
            <el-input
              v-model="input"
              type="textarea"
              :rows="4"
              resize="none"
              :placeholder="mode === 'analyze' ? '问点什么，如：分析本月销售业绩，指出问题与建议' : '描述客户需求，如：想给微胖女性推荐夏天连衣裙'"
              @keydown.ctrl.enter.prevent="send"
              @keydown.meta.enter.prevent="send"
            />
            <div class="mt-2 flex items-center justify-between">
              <span class="text-xs text-gray-400">Ctrl+Enter 发送</span>
              <el-button type="primary" :loading="streaming" :disabled="!input.trim()" @click="send">发送</el-button>
            </div>
          </div>
          <div v-if="!messages.length" class="mt-4 text-sm text-gray-500">
            我是进销存 AI 助手。输入问题开始对话，例如「分析本月销售业绩」或「想给微胖女性推荐夏天连衣裙」。
          </div>
          <div v-for="msg in messages" :key="msg.id" class="mt-4">
            <div v-if="msg.role === 'user'" class="flex justify-end">
              <div class="max-w-[85%] rounded-lg bg-blue-600 px-3 py-2 text-sm text-white dark:bg-blue-500">{{ msg.content }}</div>
            </div>
            <div v-else class="flex justify-start">
              <div class="max-w-[95%] rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900">
                <div v-if="msg.error" class="text-red-500">{{ msg.error }}</div>
                <template v-else>
                  <div v-if="msg.candidates && msg.candidates.length" class="mb-2 space-y-2">
                    <div v-for="sku in msg.candidates" :key="sku.skuId" class="flex items-center justify-between rounded border border-gray-200 px-2 py-1.5 dark:border-slate-700">
                      <div>
                        <div class="font-medium">{{ sku.goodsName }}（{{ sku.color }} {{ sku.size }}）</div>
                        <div class="text-xs text-gray-400">¥{{ sku.salePrice.toFixed(2) }} · 库存 {{ sku.available }}</div>
                      </div>
                      <el-button type="primary" size="small" plain @click="handleAddToCart(sku)">加购</el-button>
                    </div>
                  </div>
                  <div v-if="msg.displayContent" class="ai-markdown" v-html="msg.displayContent"></div>
                  <span v-if="msg.streaming" class="ai-cursor"></span>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Marked } from 'marked'
import { aiAnalyzeStream, aiRecommendStream } from '@/api/jxc/ai'
import AiAssistant from './AiAssistant.vue'

const marked = new Marked({ breaks: true, gfm: true })

const visible = ref(false)
const mode = ref('analyze')
const input = ref('')
const streaming = ref(false)
const messages = ref([])
let controller = null

let msgIdSeed = 0
const newId = () => `idx-${++msgIdSeed}-${Date.now()}`

const send = async () => {
  const text = input.value.trim()
  if (!text || streaming.value) return
  input.value = ''
  const msg = { id: newId(), role: 'assistant', content: '', displayContent: '', candidates: null, streaming: true, error: '' }
  messages.value.push({ id: newId(), role: 'user', content: text }, msg)
  streaming.value = true
  controller = new AbortController()

  try {
    if (mode.value === 'analyze') {
      let raw = ''
      await aiAnalyzeStream({ question: text, days: 0 }, {
        controller,
        onText: (t) => { raw += t; msg.displayContent = marked.parse(raw) }
      })
      msg.content = raw
    } else {
      let raw = ''
      await aiRecommendStream({ demand: text }, {
        controller,
        onEvent: ({ event, data }) => {
          if (event === 'candidates' && data) {
            msg.candidates = data.candidates || []
          } else if (event === 'message' && typeof data === 'string') {
            raw += data
            msg.displayContent = marked.parse(raw)
          }
        }
      })
      msg.content = raw
    }
  } catch (error) {
    msg.error = controller?.signal?.aborted ? '已停止生成' : (error?.message || 'AI 请求失败，请稍后重试')
  } finally {
    msg.streaming = false
    streaming.value = false
  }
}

const handleAddToCart = (sku) => {
  ElMessage.info(`SKU ${sku.skuCode} 可在收银台页加入购物车`)
}
</script>

<style scoped>
.ai-cursor {
  display: inline-block;
  width: 8px;
  height: 14px;
  margin-left: 2px;
  vertical-align: -2px;
  background: var(--el-color-primary);
  animation: ai-blink 1s step-start infinite;
}
@keyframes ai-blink {
  50% { opacity: 0; }
}
.ai-markdown :deep(p) { margin: 0.35rem 0; }
.ai-markdown :deep(ul) { list-style: disc; padding-left: 1.2rem; margin: 0.35rem 0; }
.ai-markdown :deep(ol) { list-style: decimal; padding-left: 1.2rem; margin: 0.35rem 0; }
.ai-markdown :deep(li) { margin: 0.15rem 0; }
.ai-markdown :deep(h1), .ai-markdown :deep(h2), .ai-markdown :deep(h3) {
  font-weight: 600;
  margin: 0.5rem 0 0.25rem;
}
.ai-markdown :deep(strong) { font-weight: 600; }
.ai-markdown :deep(code) {
  background: var(--el-fill-color-light);
  border-radius: 4px;
  padding: 0 4px;
  font-size: 12px;
}
.ai-markdown :deep(pre) {
  background: var(--el-fill-color-light);
  border-radius: 6px;
  padding: 8px;
  overflow-x: auto;
}
.ai-markdown :deep(pre code) { background: transparent; padding: 0; }
</style>
