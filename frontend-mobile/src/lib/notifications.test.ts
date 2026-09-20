import { describe, expect, it } from 'vitest'
import { groupNotificationsByDate } from './notifications'
import type { Notification } from '@/types'

function n(id: string, created_at: string): Notification {
  return {
    id,
    title: `t-${id}`,
    body: 'b',
    category: 'system',
    is_read: false,
    read_at: null,
    created_at,
  }
}

describe('groupNotificationsByDate', () => {
  const now = new Date('2026-09-20T10:00:00.000Z')

  it('按日期倒序分组，并给出今天/昨天/具体日期三种标签', () => {
    const groups = groupNotificationsByDate(
      [
        n('1', '2026-09-20T09:00:00.000Z'),
        n('2', '2026-09-19T09:00:00.000Z'),
        n('3', '2026-09-01T09:00:00.000Z'),
        n('4', '2026-09-20T08:00:00.000Z'),
      ],
      now,
    )

    expect(groups.map((g) => g.label)).toEqual(['今天', '昨天', '09月01日'])
    expect(groups[0]?.items.map((x) => x.id)).toEqual(['1', '4'])
    expect(groups[1]?.items.map((x) => x.id)).toEqual(['2'])
  })

  it('空输入返回空数组', () => {
    expect(groupNotificationsByDate([], now)).toEqual([])
  })
})
