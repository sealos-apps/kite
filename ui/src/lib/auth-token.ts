export const AUTH_TOKEN_STORAGE_KEY = 'kite-auth-token'

const isBrowser = (): boolean => typeof window !== 'undefined'

export const readAuthToken = (): string | null => {
  if (!isBrowser()) return null
  return sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY)
}

export const writeAuthToken = (token: string | null): void => {
  if (!isBrowser()) return
  if (token && token.trim()) {
    sessionStorage.setItem(AUTH_TOKEN_STORAGE_KEY, token.trim())
    return
  }
  sessionStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
}
