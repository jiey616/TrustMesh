import { useContext } from 'react'
import { RealtimeStatusContext } from '../context'

export function useRealtimeStatus() {
  return useContext(RealtimeStatusContext)
}
