import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as meetingsApi from '@/api/meetings'
import type { MeetingAgendaItem, MeetingParticipant } from '@/types/meeting'

export function useMeetings(projectId: string) {
  return useQuery({
    queryKey: ['meetings', projectId],
    queryFn: () => meetingsApi.listMeetings(projectId),
  })
}

export function useMeeting(meetingId: string | undefined) {
  return useQuery({
    queryKey: ['meeting', meetingId],
    queryFn: () => meetingsApi.getMeeting(meetingId!),
    enabled: !!meetingId,
  })
}

export function useMeetingMessages(meetingId: string | undefined) {
  return useQuery({
    queryKey: ['meeting-messages', meetingId],
    queryFn: () => meetingsApi.listMeetingMessages(meetingId!),
    enabled: !!meetingId,
    refetchInterval: 3_000, // Poll every 3s for new messages
  })
}

export function useCreateMeeting(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      title: string
      agenda: string
      host_agent_id: string
      participants: MeetingParticipant[]
      agenda_items?: MeetingAgendaItem[]
      file_ids?: string[]
    }) =>
      meetingsApi.createMeeting(projectId, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['meetings', projectId] })
    },
  })
}
