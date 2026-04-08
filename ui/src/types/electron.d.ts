export {}

declare global {
  interface Window {
    kiteDesktop?: {
      openFiles: () => Promise<{
        canceled: boolean
        files: Array<{
          path: string
          name: string
          content: string
        }>
      }>
    }
  }
}
