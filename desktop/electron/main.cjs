const fs = require('node:fs')
const http = require('node:http')
const net = require('node:net')
const path = require('node:path')
const { spawn } = require('node:child_process')

const { app, BrowserWindow, dialog } = require('electron')

const START_PORT = Number.parseInt(process.env.KITE_DESKTOP_PORT || '18680', 10)
const PORT_SCAN_LIMIT = 80
const BACKEND_READY_TIMEOUT_MS = 30_000

let mainWindow = null
let backendProcess = null
let backendPort = null
let isShuttingDown = false

function getBinaryName() {
  return process.platform === 'win32' ? 'kite.exe' : 'kite'
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
    server.listen(port, '127.0.0.1', () => {
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

async function waitBackendReady(port, timeoutMs) {
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    const isHealthy = await requestHealth(port)
    if (isHealthy) {
      return
    }
    await wait(300)
  }
  throw new Error(`Backend did not pass health check within ${timeoutMs}ms`)
}

function pipeBackendLogs(proc) {
  if (proc.stdout) {
    proc.stdout.on('data', (chunk) => {
      process.stdout.write(`[kite] ${chunk}`)
    })
  }

  if (proc.stderr) {
    proc.stderr.on('data', (chunk) => {
      process.stderr.write(`[kite] ${chunk}`)
    })
  }
}

async function startBackend() {
  const binaryPath = resolveBackendBinaryPath()
  if (!fs.existsSync(binaryPath)) {
    throw new Error(`Backend binary not found: ${binaryPath}`)
  }

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
  }

  backendProcess = spawn(binaryPath, [], {
    cwd: path.dirname(binaryPath),
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  pipeBackendLogs(backendProcess)

  backendProcess.once('exit', (code, signal) => {
    if (!isShuttingDown) {
      const reason =
        typeof code === 'number'
          ? `Backend exited with code ${code}`
          : `Backend terminated by signal ${signal || 'unknown'}`
      dialog.showErrorBox('Kite backend stopped', reason)
      app.quit()
    }
  })

  await waitBackendReady(backendPort, BACKEND_READY_TIMEOUT_MS)
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
  mainWindow = new BrowserWindow({
    width: 1440,
    height: 920,
    minWidth: 1024,
    minHeight: 720,
    show: false,
    autoHideMenuBar: true,
    title: 'Kite',
    webPreferences: {
      contextIsolation: true,
      sandbox: true,
    },
  })

  mainWindow.once('ready-to-show', () => {
    mainWindow.show()
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })

  mainWindow.loadURL(`http://127.0.0.1:${backendPort}`)
}

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
