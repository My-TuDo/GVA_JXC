<template>
  <el-drawer
    v-model="visible"
    :title="`AI 智能助手 · ${modeLabel}`"
    size="500px"
    append-to-body
    :destroy-on-close="false"
  >
    <!-- 模式切换 -->
    <div class="mb-3">
      <el-radio-group v-model="mode" size="small">
        <el-radio-button value="analyze">销售业绩分析</el-radio-button>
        <el-radio-button value="recommend">客户需求推荐</el-radio-button>
      </el-radio-group>
    </div>

    <!-- 消息区 -->
    <div ref="msgBoxRef" class="ai-msg-box rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-slate-700 dark:bg-slate-800">
      <div v-if="!messages.length" class="text-sm text-gray-500 dark:text-slate-400">
        <div class="mb-2">我是进销存 AI 助手，可以：</div>
        <ul class="list-disc pl-4 space-y-1">
          <li>分析销售业绩：汇总趋势、热销、毛利，给出经营建议</li>
          <li>客户需求推荐：描述客户需求，我帮你挑合适的商品</li>
        </ul>
        <div class="mt-3 flex flex-wrap gap-2">
          <el-tag
            v-for="chip in quickChips"
            :key="chip"
            class="cursor-pointer"
            :class="{ 'recommend-tag': mode === 'recommend' }"
            @click="sendChip(chip)"
          >{{ chip }}</el-tag>
        </div>
      </div>

      <div
        v-for="msg in messages"
        :key="msg.id"
        class="mb-3"
      >
        <!-- 用户消息 -->
        <div v-if="msg.role === 'user'" class="flex justify-end">
          <div class="max-w-[85%] rounded-lg bg-blue-600 px-3 py-2 text-sm text-white dark:bg-blue-500">
            {{ msg.content }}
          </div>
        </div>

        <!-- 助手消息 -->
        <div v-else class="flex justify-start">
          <div class="max-w-[95%] rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900">
            <div v-if="msg.error" class="text-red-500">{{ msg.error }}</div>
            <template v-else>
              <!-- 推荐候选商品 -->
              <div v-if="msg.candidates && msg.candidates.length" class="mb-2">
                <div class="mb-1 text-xs text-gray-400">候选商品 {{ msg.candidates.length }} 件（按近30天销量排序）</div>
                <div class="space-y-2">
                  <div
                    v-for="sku in msg.candidates"
                    :key="sku.skuId"
                    class="flex items-center justify-between rounded border border-gray-200 px-2 py-1.5 dark:border-slate-700"
                  >
                    <div class="min-w-0">
                      <div class="truncate font-medium">{{ sku.goodsName }}（{{ sku.color }} {{ sku.size }}）</div>
                      <div class="text-xs text-gray-400">¥{{ sku.salePrice.toFixed(2) }} · 库存 {{ sku.available }}</div>
                    </div>
                    <el-button type="primary" size="small" plain @click="handleAddToCart(sku)">加购</el-button>
                  </div>
                </div>
              </div>
              <!-- markdown 正文 -->
              <div v-if="msg.displayContent" class="ai-markdown" v-html="msg.displayContent"></div>
              <span v-if="msg.streaming" class="ai-cursor"></span>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- 输入区 -->
    <div class="mt-3">
      <el-input
        v-model="input"
        type="textarea"
        :rows="3"
        resize="none"
        :placeholder="mode === 'analyze' ? '问点什么，如：分析本月销售业绩，指出问题与建议' : '描述客户需求，如：想给微胖女性推荐夏天连衣裙'"
        @keydown.ctrl.enter.prevent="send"
        @keydown.meta.enter.prevent="send"
      />
      <div class="mt-2 flex items-center justify-between">
        <span class="text-xs text-gray-400">Ctrl+Enter 发送</span>
        <div class="flex gap-2">
          <el-button v-if="streaming" type="danger" size="small" @click="stopStreaming">停止生成</el-button>
          <el-button type="primary" size="small" :loading="streaming" :disabled="!input.trim()" @click="send">发送</el-button>
        </div>
      </div>
    </div>
  </el-drawer>
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { Marked } from 'marked'
import { aiAnalyzeStream, aiRecommendStream } from '@/api/jxc/ai'

const marked = new Marked({ breaks: true, gfm: true })

const props = defineProps({
  // 初始模式：analyze 业绩分析 / recommend 商品推荐
  initialMode: { type: String, default: 'analyze' },
  // 收银台页传入：加购回调（sku）→ 返回 true 表示已加购
  addToCart: { type: Function, default: null }
})

const visible = defineModel('visible', { type: Boolean, default: false })

const mode = ref(props.initialMode)
const modeLabel = computed(() => (mode.value === 'analyze' ? '销售业绩分析' : '客户需求推荐'))

const quickChips = computed(() =>
  mode.value === 'analyze'
    ? ['分析本月销售业绩', '今天卖得怎么样？有什么要注意的', '最近什么好卖？备货建议']
    : ['想给微胖女性推荐夏天连衣裙', '推荐几件100元左右的上衣', '给40岁男士推荐商务休闲裤']
)

let msgIdSeed = 0
const newId = () => `msg-${++msgIdSeed}-${Date.now()}`

const messages = ref([])
const input = ref('')
const streaming = ref(false)
const msgBoxRef = ref(null)
let controller = null

const scrollToBottom = async () => {
  await nextTick()
  if (msgBoxRef.value) msgBoxRef.value.scrollTop = msgBoxRef.value.scrollHeight
}

// 追加用户消息与占位助手消息，返回助手消息引用
const pushPair = (text) => {
  const userMsg = { id: newId(), role: 'user', content: text }
  const assistantMsg = {
    id: newId(),
    role: 'assistant',
    content: '',
    displayContent: '',
    candidates: null,
    conditions: null,
    streaming: true,
    error: ''
  }
  messages.value.push(userMsg, assistantMsg)
  return assistantMsg
}

const finish = (msg, errorText = '') => {
  msg.streaming = false
  if (errorText) msg.error = errorText
  streaming.value = false
  scrollToBottom()
}

const sendChip = (text) => {
  if (streaming.value) return
  input.value = text
  send()
}

const send = async () => {
  const text = input.value.trim()
  if (!text || streaming.value) return
  input.value = ''
  const msg = pushPair(text)
  streaming.value = true
  controller = new AbortController()
  scrollToBottom()

  try {
    if (mode.value === 'analyze') {
      // 业绩分析：后端按问题中的时间词（上月/今年/近7天等）聚合数据，流式返回解读
      let raw = ''
      await aiAnalyzeStream(
        { question: text, days: 0 },
        {
          controller,
          onText: (t) => {
            raw += t
            msg.displayContent = marked.parse(raw)
          }
        }
      )
      msg.content = raw
    } else {
      // 商品推荐：先 candidates 事件（结构化候选），再流式推荐语
      let raw = ''
      await aiRecommendStream(
        { demand: text },
        {
          controller,
          onEvent: ({ event, data }) => {
            if (event === 'candidates' && data) {
              msg.conditions = data.conditions || null
              msg.candidates = data.candidates || []
              scrollToBottom()
            } else if (event === 'message' && typeof data === 'string') {
              raw += data
              msg.displayContent = marked.parse(raw)
              scrollToBottom()
            }
          }
        }
      )
      msg.content = raw
    }
    finish(msg)
  } catch (error) {
    if (controller?.signal?.aborted) {
      finish(msg, '已停止生成')
    } else {
      finish(msg, error?.message || 'AI 请求失败，请稍后重试')
    }
  }
}

const stopStreaming = () => {
  controller?.abort()
}

const handleAddToCart = (sku) => {
  if (props.addToCart) {
    const ok = props.addToCart(sku)
    if (ok) ElMessage.success(`已加入购物车：${sku.goodsName}（${sku.color} ${sku.size}）`)
  } else {
    ElMessage.info(`SKU ${sku.skuCode} 可在收银台页加入购物车`)
  }
}

// 打开时自动聚焦输入
watch(visible, (v) => {
  if (v) nextTick(() => scrollToBottom())
})
</script>

<style scoped>
.ai-msg-box {
  height: calc(100vh - 300px);
  min-height: 320px;
  overflow-y: auto;
}

/* 流式输出光标 */
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

/* markdown 渲染（:deep 覆盖内部元素，见 AGENT.MD 样式规范） */
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
