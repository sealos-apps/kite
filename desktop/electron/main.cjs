const fs = require('node:fs')
const http = require('node:http')
const net = require('node:net')
const path = require('node:path')
const { spawn, spawnSync } = require('node:child_process')

const { app, BrowserWindow, dialog, ipcMain } = require('electron')

const START_PORT = Number.parseInt(process.env.KITE_DESKTOP_PORT || '18680', 10)
const PORT_SCAN_LIMIT = 80
const BACKEND_READY_TIMEOUT_MS = Number.parseInt(
  process.env.KITE_DESKTOP_BACKEND_READY_TIMEOUT_MS || '120000',
  10
)
const WINDOW_LOAD_RETRY_MAX = Number.parseInt(
  process.env.KITE_DESKTOP_WINDOW_LOAD_RETRY_MAX || '8',
  10
)
const WINDOW_LOAD_RETRY_DELAY_MS = Number.parseInt(
  process.env.KITE_DESKTOP_WINDOW_LOAD_RETRY_DELAY_MS || '500',
  10
)

let mainWindow = null
let backendProcess = null
let backendPort = null
let isShuttingDown = false
let backendReady = false
let windowLoadRetryCount = 0
let didShowWindowLoadError = false

function resolveRuntimeIconPath() {
  return path.resolve(__dirname, '..', 'icons', 'icon.png')
}

function getRuntimeIconPath() {
  const iconPath = resolveRuntimeIconPath()
  if (fs.existsSync(iconPath)) {
    return iconPath
  }
  return null
}

function getBinaryName() {
  return process.platform === 'win32' ? 'kite.exe' : 'kite'
}

function canUseExecutablePath(filePath) {
  try {
    fs.accessSync(filePath, fs.constants.X_OK)
    return true
  } catch {
    return false
  }
}

function resolveBackendBinaryPath() {
  if (process.env.KITE_DESKTOP_BACKEND) {
    return path.resolve(process.env.KITE_DESKTOP_BACKEND)
  }

  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'backend', getBinaryName())
  }

  return path.resolve(__dirname, '..', '..', getBinaryName())
}

function clearMacQuarantine(filePath) {
  if (process.platform !== 'darwin') return
  spawnSync('xattr', ['-dr', 'com.apple.quarantine', filePath], {
    stdio: 'ignore',
  })
}

function ensureRunnableBackendBinary(sourceBinaryPath) {
  if (!app.isPackaged) {
    return sourceBinaryPath
  }

  const runtimeBinDir = path.join(app.getPath('userData'), 'runtime-bin')
  fs.mkdirSync(runtimeBinDir, { recursive: true })
  const runtimeBinaryPath = path.join(runtimeBinDir, getBinaryName())

  const shouldCopy = (() => {
    if (!fs.existsSync(runtimeBinaryPath)) {
      return true
    }
    try {
      const srcStat = fs.statSync(sourceBinaryPath)
      const dstStat = fs.statSync(runtimeBinaryPath)
      return srcStat.size !== dstStat.size || srcStat.mtimeMs > dstStat.mtimeMs
    } catch {
      return true
    }
  })()

  if (shouldCopy) {
    fs.copyFileSync(sourceBinaryPath, runtimeBinaryPath)
  }

  if (process.platform !== 'win32') {
    fs.chmodSync(runtimeBinaryPath, 0o755)
  }
  clearMacQuarantine(runtimeBinaryPath)

  if (!canUseExecutablePath(runtimeBinaryPath)) {
    throw new Error(`Backend binary is not executable: ${runtimeBinaryPath}`)
  }
  return runtimeBinaryPath
}

function wait(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms)
  })
}

function isPortAvailable(port) {
  return new Promise((resolve) => {
    const server = net.createServer()
    server.unref()
    server.once('error', () => resolve(false))
    server.listen(port, () => {
      server.close(() => resolve(true))
    })
  })
}

async function findAvailablePort(startPort) {
  for (let port = startPort; port < startPort + PORT_SCAN_LIMIT; port++) {
    if (await isPortAvailable(port)) {
      return port
    }
  }
  throw new Error(`No available port in range ${startPort}-${startPort + PORT_SCAN_LIMIT - 1}`)
}

function requestHealth(port) {
  return new Promise((resolve) => {
    const req = http.get(
      {
        host: '127.0.0.1',
        port,
        path: '/healthz',
        timeout: 1500,
      },
      (res) => {
        res.resume()
        resolve(res.statusCode === 200)
      }
    )

    req.on('error', () => resolve(false))
    req.on('timeout', () => {
      req.destroy()
      resolve(false)
    })
  })
}

async function waitBackendReady(port, timeoutMs, getFailureDetails) {
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    const failureDetails = getFailureDetails()
    if (failureDetails) {
      throw new Error(failureDetails)
    }

    const isHealthy = await requestHealth(port)
    if (isHealthy) {
      return
    }
    await wait(300)
  }
  const failureDetails = getFailureDetails()
  const detailsSuffix = failureDetails ? ` (${failureDetails})` : ''
  throw new Error(
    `Backend did not pass health check within ${timeoutMs}ms${detailsSuffix}`
  )
}

function pipeBackendLogs(proc, logTail) {
  const appendLog = (prefix, chunk) => {
    const text = chunk.toString()
    logTail.push(`${prefix}${text}`)
    const joined = logTail.join('')
    if (joined.length > 6000) {
      const trimmed = joined.slice(joined.length - 6000)
      logTail.length = 0
      logTail.push(trimmed)
    }
  }

  if (proc.stdout) {
    proc.stdout.on('data', (chunk) => {
      appendLog('[stdout] ', chunk)
      process.stdout.write(`[kite] ${chunk}`)
    })
  }

  if (proc.stderr) {
    proc.stderr.on('data', (chunk) => {
      appendLog('[stderr] ', chunk)
      process.stderr.write(`[kite] ${chunk}`)
    })
  }
}

async function startBackend() {
  const sourceBinaryPath = resolveBackendBinaryPath()
  if (!fs.existsSync(sourceBinaryPath)) {
    throw new Error(`Backend binary not found: ${sourceBinaryPath}`)
  }

  const binaryPath = ensureRunnableBackendBinary(sourceBinaryPath)
  backendPort = await findAvailablePort(START_PORT)
  const userDataDir = app.getPath('userData')
  fs.mkdirSync(userDataDir, { recursive: true })

  const dbPath = path.join(userDataDir, 'kite.db')
  const env = {
    ...process.env,
    PORT: String(backendPort),
    HOST: '127.0.0.1',
    DB_DSN: dbPath,
    DISABLE_VERSION_CHECK: 'true',
    AUTH_COOKIE_SECURE: 'false',
    KITE_DESKTOP_MODE: 'true',
  }

  backendProcess = spawn(binaryPath, [], {
    cwd: path.dirname(binaryPath),
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  const logTail = []
  let spawnError = null
  let earlyExitReason = null
  backendReady = false

  backendProcess.once('error', (error) => {
    spawnError = `Failed to spawn backend: ${error.message}`
  })

  pipeBackendLogs(backendProcess, logTail)

  backendProcess.once('exit', (code, signal) => {
    const reason =
      typeof code === 'number'
        ? `Backend exited with code ${code}`
        : `Backend terminated by signal ${signal || 'unknown'}`
    if (!backendReady) {
      earlyExitReason = reason
      return
    }

    if (!isShuttingDown) {
      dialog.showErrorBox('Kite backend stopped', reason)
      app.quit()
    }
  })

  await waitBackendReady(backendPort, BACKEND_READY_TIMEOUT_MS, () => {
    if (spawnError) {
      return spawnError
    }
    if (earlyExitReason) {
      return `${earlyExitReason}. Recent logs: ${logTail.join('').trim()}`
    }
    return null
  })
  backendReady = true
}

function terminateProcessTree(pid, signal = 'SIGTERM') {
  if (!pid) return
  if (process.platform === 'win32') {
    spawn('taskkill', ['/pid', String(pid), '/t', '/f'])
    return
  }
  try {
    process.kill(pid, signal)
  } catch {
    // ignore
  }
}

async function stopBackend() {
  if (!backendProcess) return
  const proc = backendProcess
  backendProcess = null
  backendReady = false

  await new Promise((resolve) => {
    const timeout = setTimeout(() => {
      terminateProcessTree(proc.pid, 'SIGKILL')
      resolve()
    }, 4000)

    proc.once('exit', () => {
      clearTimeout(timeout)
      resolve()
    })

    terminateProcessTree(proc.pid)
  })
}

function createMainWindow() {
  const runtimeIconPath = getRuntimeIconPath()
  windowLoadRetryCount = 0
  didShowWindowLoadError = false
  mainWindow = new BrowserWindow({
    width: 1440,
    height: 920,
    minWidth: 1024,
    minHeight: 720,
    icon: runtimeIconPath || undefined,
    show: false,
    autoHideMenuBar: true,
    title: 'Kite',
    webPreferences: {
      contextIsolation: true,
      sandbox: true,
      preload: path.join(__dirname, 'preload.cjs'),
    },
  })

  mainWindow.once('ready-to-show', () => {
    mainWindow.show()
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })

  mainWindow.webContents.on(
    'did-fail-load',
    async (_event, errorCode, errorDescription, validatedURL, isMainFrame) => {
      if (!isMainFrame || !mainWindow || mainWindow.isDestroyed()) {
        return
      }

      const isBackendConnectionIssue =
        errorCode === -102 ||
        errorCode === -105 ||
        errorCode === -106 ||
        errorCode === -118 ||
        errorCode === -120

      if (
        isBackendConnectionIssue &&
        windowLoadRetryCount < WINDOW_LOAD_RETRY_MAX
      ) {
        windowLoadRetryCount += 1
        await wait(WINDOW_LOAD_RETRY_DELAY_MS)

        // Try to ensure backend health before retrying window load.
        if (backendPort) {
          await requestHealth(backendPort)
        }
        if (!mainWindow || mainWindow.isDestroyed()) {
          return
        }
        mainWindow.loadURL(`http://127.0.0.1:${backendPort}`)
        return
      }

      if (didShowWindowLoadError || isShuttingDown) {
        return
      }
      didShowWindowLoadError = true

      const message = [
        `Failed to load ${validatedURL || `http://127.0.0.1:${backendPort}`}.`,
        `Error: ${errorDescription} (code ${errorCode})`,
        `Retries: ${windowLoadRetryCount}/${WINDOW_LOAD_RETRY_MAX}`,
      ].join('\n')
      dialog.showErrorBox('Kite failed to load UI', message)
      app.quit()
    }
  )

  mainWindow.loadURL(`http://127.0.0.1:${backendPort}`)
}

ipcMain.handle('kite-desktop:pick-files', async () => {
  const targetWindow = mainWindow && !mainWindow.isDestroyed() ? mainWindow : null
  const result = await dialog.showOpenDialog(targetWindow, {
    properties: ['openFile', 'multiSelections'],
    title: 'Select kubeconfig files',
  })

  if (result.canceled) {
    return {
      canceled: true,
      files: [],
    }
  }
  const files = await Promise.all(
    (result.filePaths || []).map(async (filePath) => {
      const content = await fs.promises.readFile(filePath, 'utf8')
      return {
        path: filePath,
        name: path.basename(filePath),
        content,
      }
    })
  )
  return {
    canceled: false,
    files,
  }
})

async function boot() {
  try {
    await startBackend()
    createMainWindow()
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    dialog.showErrorBox('Failed to start Kite desktop', message)
    app.quit()
  }
}

app.on('before-quit', (event) => {
  if (isShuttingDown) {
    return
  }

  event.preventDefault()
  isShuttingDown = true
  void stopBackend().finally(() => {
    app.exit(0)
  })
})

app.whenReady().then(() => {
  const runtimeIconPath = getRuntimeIconPath()
  if (
    process.platform === 'darwin' &&
    runtimeIconPath &&
    app.dock &&
    typeof app.dock.setIcon === 'function'
  ) {
    app.dock.setIcon(runtimeIconPath)
  }
  void boot()
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0 && backendPort) {
    createMainWindow()
  }
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
