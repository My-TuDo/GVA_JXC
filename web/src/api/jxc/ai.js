import { useUserStore } from '@/pinia/modules/user'

const AI_TIMEOUT = 1000 * 60 * 5

const timerHost = typeof window !== 'undefined' ? window : globalThis

// 构造带鉴权的 SSE 请求头（EventSource 不支持自定义 header，故用 fetch）
const createAIHeaders = () => {
  const userStore = useUserStore()
  return {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
    'x-token': userStore.token || '',
    'x-user-id': userStore.userInfo?.ID || userStore.userInfo?.id || ''
  }
}

// 解析非 SSE 响应体为 JSON（失败时原样返回文本）
const parseFetchBody = async (response) => {
  const text = await response.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch (e) {
    return { code: -1, msg: text }
  }
}

// 由 fetch 响应构造带上游错误信息的 Error
const buildFetchError = async (response) => {
  const body = await parseFetchBody(response)
  const message =
    (body && (body.msg || body.message)) ||
    `AI request failed: HTTP ${response.status}`
  return new Error(message)
}

/**
 * JXC AI SSE 请求核心
 * 后端约定：event: <name> 行 + data: <json|text> 行，结束 data: [DONE]
 * 事件：candidates（{conditions, candidates}）、message（纯文本增量）、error
 * @param {object} options.onEvent 收到完整事件回调 ({event, data})
 * @param {object} options.onText   纯文本增量回调 (text)
 * @param {object} options.controller 外部 AbortController（可中止）
 */
const aiSSERequest = async (url, data, options = {}) => {
  const controller = options.controller || new AbortController()
  const timeout = options.timeout ?? AI_TIMEOUT
  let timeoutId = null
  let timeoutTriggered = false

  if (timeout > 0) {
    timeoutId = timerHost.setTimeout(() => {
      timeoutTriggered = true
      controller.abort()
    }, timeout)
  }

  try {
    const fetchUrl = `${import.meta.env.VITE_BASE_API || ''}${url}`
    const response = await fetch(fetchUrl, {
      method: 'POST',
      headers: createAIHeaders(),
      body: JSON.stringify(data),
      signal: controller.signal
    })

    if (!response.ok) {
      throw await buildFetchError(response)
    }

    const contentType = String(
      response.headers.get('content-type') || ''
    ).toLowerCase()
    // 非 SSE（如参数/鉴权错误）：降级为普通 JSON 解析
    if (!contentType.includes('text/event-stream') || !response.body) {
      const body = await parseFetchBody(response)
      if (body && typeof body.code !== 'undefined' && body.code !== 0) {
        throw new Error(body.msg || 'AI request failed')
      }
      return body
    }

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let pendingEvent = 'message'
    let done = false

    const dispatch = (eventName, payload) => {
      // 容错：无 event 行的文本流在部分实现里可能被标成 'data'，归一为 message
      if (eventName === 'data') eventName = 'message'
      if (eventName === 'error') {
        throw new Error(
          (typeof payload === 'object' && (payload.msg || payload.message)) ||
            String(payload) ||
            'AI 服务异常'
        )
      }
      options.onEvent?.({ event: eventName, data: payload })
      if (eventName === 'message' && typeof payload === 'string' && payload) {
        options.onText?.(payload)
      }
    }

    while (!done) {
      const result = await reader.read()
      done = result.done
      if (done) break

      buffer += decoder.decode(result.value, { stream: true })
      const lines = buffer.split('\n')
      // 保留最后一条可能不完整的行
      buffer = lines.pop() || ''

      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed) continue
        if (trimmed.startsWith('event:')) {
          pendingEvent = trimmed.slice(6).trim()
          continue
        }
        if (trimmed.startsWith('data:')) {
          const dataStr = trimmed.slice(5).trim()
          if (!dataStr || dataStr === '[DONE]') continue
          let payload = dataStr
          try {
            payload = JSON.parse(dataStr)
          } catch (e) {
            // 非 JSON 裸文本
          }
          dispatch(pendingEvent, payload)
        }
      }
    }

    // EOF 后冲刷残留
    if (buffer.trim()) {
      const trimmed = buffer.trim()
      if (trimmed.startsWith('event:')) {
        pendingEvent = trimmed.slice(6).trim()
      } else if (trimmed.startsWith('data:')) {
        const dataStr = trimmed.slice(5).trim()
        if (dataStr && dataStr !== '[DONE]') {
          let payload = dataStr
          try {
            payload = JSON.parse(dataStr)
          } catch (e) {
            // 裸文本
          }
          dispatch(pendingEvent, payload)
        }
      }
    }
    return true
  } catch (error) {
    if (controller.signal.aborted && timeoutTriggered) {
      throw new Error('AI 请求超时')
    }
    throw error
  } finally {
    if (timeoutId) timerHost.clearTimeout(timeoutId)
  }
}

// AI 销售业绩分析（流式）
export const aiAnalyzeStream = (data, options = {}) =>
  aiSSERequest('/jxc/ai/analyze', data, options)

// AI 客户需求商品推荐（先 candidates 事件，再流式推荐语）
export const aiRecommendStream = (data, options = {}) =>
  aiSSERequest('/jxc/ai/recommend', data, options)
