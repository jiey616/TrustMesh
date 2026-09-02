import { apiClient } from './client'
import type { Meeting, MeetingMessage, MeetingParticipant, MeetingAgendaItem } from '@/types'

export async function createMeeting(
  projectId: string,
  data: {
    title: string
    agenda: string
    host_agent_id: string
    participants: MeetingParticipant[]
    agenda_items?: MeetingAgendaItem[]
    file_ids?: string[]
  },
): Promise<Meeting> {
  const res = await apiClient.post(`/api/v1/projects/${projectId}/meetings`, { json: data })
  return (await res.json<{ data: Meeting }>()).data
}

export async function listMeetings(projectId: string): Promise<Meeting[]> {
  const res = await apiClient.get(`/api/v1/projects/${projectId}/meetings`)
  const body = await res.json<{ data: { items: Meeting[] } }>()
  return body.data.items
}

export async function getMeeting(meetingId: string): Promise<Meeting> {
  const res = await apiClient.get(`/api/v1/meetings/${meetingId}`)
  return (await res.json<{ data: Meeting }>()).data
}

export async function startMeeting(meetingId: string): Promise<void> {
  await apiClient.post(`/api/v1/meetings/${meetingId}/start`)
}

export async function endMeeting(meetingId: string): Promise<void> {
  await apiClient.post(`/api/v1/meetings/${meetingId}/end`)
}

export async function sendMeetingMessage(meetingId: string, content: string): Promise<MeetingMessage> {
  const res = await apiClient.post(`/api/v1/meetings/${meetingId}/messages`, { json: { content } })
  return (await res.json<{ data: MeetingMessage }>()).data
}

export async function listMeetingMessages(meetingId: string): Promise<MeetingMessage[]> {
  const res = await apiClient.get(`/api/v1/meetings/${meetingId}/messages`)
  const body = await res.json<{ data: { items: MeetingMessage[] } }>()
  return body.data.items
}
