import { api } from './client'
import type { Meeting, MeetingMessage, MeetingParticipant, MeetingAgendaItem } from '@/types/meeting'

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
  const res = await api.post(`projects/${projectId}/meetings`, { json: data })
  return (await res.json<{ data: Meeting }>()).data
}

export async function listMeetings(projectId: string): Promise<Meeting[]> {
  const res = await api.get(`projects/${projectId}/meetings`)
  const body = await res.json<{ data: { items: Meeting[] } }>()
  return body.data.items
}

export async function getMeeting(meetingId: string): Promise<Meeting> {
  const res = await api.get(`meetings/${meetingId}`)
  return (await res.json<{ data: Meeting }>()).data
}

export async function startMeeting(meetingId: string): Promise<void> {
  await api.post(`meetings/${meetingId}/start`)
}

export async function endMeeting(meetingId: string): Promise<void> {
  await api.post(`meetings/${meetingId}/end`)
}

export async function sendMeetingMessage(meetingId: string, content: string): Promise<MeetingMessage> {
  const res = await api.post(`meetings/${meetingId}/messages`, { json: { content } })
  return (await res.json<{ data: MeetingMessage }>()).data
}

export async function listMeetingMessages(meetingId: string): Promise<MeetingMessage[]> {
  const res = await api.get(`meetings/${meetingId}/messages`)
  const body = await res.json<{ data: { items: MeetingMessage[] } }>()
  return body.data.items
}
