import type { QueryClient } from '@tanstack/react-query'
import type { Agent, Event } from '@/types'
import { applyProjectAgentStatusChanged } from './projects'

const DEFAULT_AGENT_EVENTS_LIMIT = 50

function sortEventsDescending(items: Event[]) {
  return items.slice().sort((left, right) => right.created_at.localeCompare(left.created_at))
}

function sortAgentsByName(items: Agent[]) {
  return items.slice().sort((left, right) => left.name.localeCompare(right.name))
}

export function applyAgentStatusChanged(
  queryClient: QueryClient,
  payload: { agent: Agent; event: Event }
) {
  queryClient.setQueryData<Agent[] | undefined>(['agents'], (items) => {
    if (!items) {
      return items
    }
    const index = items.findIndex((item) => item.id === payload.agent.id)
    if (index < 0) {
      return sortAgentsByName([payload.agent, ...items])
    }
    const next = items.slice()
    next[index] = payload.agent
    return sortAgentsByName(next)
  })

  queryClient.setQueryData(['agents', payload.agent.id], payload.agent)

  queryClient.setQueryData<Event[] | undefined>(
    ['agents', payload.agent.id, 'events', DEFAULT_AGENT_EVENTS_LIMIT],
    (items) => {
      if (!items) {
        return items
      }
      if (items.some((item) => item.id === payload.event.id)) {
        return items
      }
      return sortEventsDescending([payload.event, ...items]).slice(0, DEFAULT_AGENT_EVENTS_LIMIT)
    }
  )

  applyProjectAgentStatusChanged(queryClient, payload.agent)
  void queryClient.invalidateQueries({ queryKey: ['tasks'] })
  void queryClient.invalidateQueries({ queryKey: ['dashboard', 'stats'] })
}
