/**
 * 剥离消息开头的 "[reply]"/"[response]" 协议发布标记，保留其后真实内容。
 * ClawSynapse 数字员工在发送出站消息时可能给内容加该标记（如 "[reply] 好的，马上处理"），
 * 属于协议装饰，不应展示给用户。按行处理以兼容多行消息。
 */
export function stripReplyPrefix(text: string): string {
  if (!text) return text
  return text
    .split('\n')
    .map((line) => line.replace(/^\s*\[(?:reply|response)\]\s*/i, ''))
    .join('\n')
}
