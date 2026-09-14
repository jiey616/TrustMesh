const explicitBaseUrl = import.meta.env.VITE_API_BASE_URL?.trim()

export const apiBaseUrl = explicitBaseUrl
  ? explicitBaseUrl.replace(/\/$/, '')
  : '/api/v1'

export function apiUrl(path: string) {
  return `${apiBaseUrl}/${path.replace(/^\/+/, '')}`
}
