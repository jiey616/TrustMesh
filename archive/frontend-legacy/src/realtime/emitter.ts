import type { RealtimeEvent } from './types'

// 模块级实时事件广播：RealtimeProvider 收到 SSE 消息后 emit，
// 需要原始事件的消费者（如办公室可视化页）订阅。
// 刻意不走 React Context，避免给所有页面增加重渲染。
type RealtimeListener = (event: RealtimeEvent) => void

const listeners = new Set<RealtimeListener>()

export function emitRealtimeEvent(event: RealtimeEvent) {
  for (const listener of listeners) {
    try {
      listener(event)
    } catch (err) {
      // 单个消费者异常不能影响其他消费者和 reducers
      console.error('[realtime] listener error', err)
    }
  }
}

export function subscribeRealtimeEvents(listener: RealtimeListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
