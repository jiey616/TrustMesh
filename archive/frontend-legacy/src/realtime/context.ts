import { createContext } from 'react'

export type RealtimeStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

export const RealtimeStatusContext = createContext<RealtimeStatus>('idle')
