const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('kiteDesktop', {
  openFiles: async () => {
    return ipcRenderer.invoke('kite-desktop:pick-files')
  },
})
