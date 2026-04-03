import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const desktopRoot = path.resolve(__dirname, '..')
const repoRoot = path.resolve(desktopRoot, '..')
const uiRoot = path.join(repoRoot, 'ui')
const backendDir = path.join(desktopRoot, 'backend')
const backendBinary = path.join(
  backendDir,
  process.platform === 'win32' ? 'kite.exe' : 'kite'
)

function run(cmd, args, options) {
  const result = spawnSync(cmd, args, {
    stdio: 'inherit',
    ...options,
  })

  if (result.status !== 0) {
    const commandString = `${cmd} ${args.join(' ')}`
    throw new Error(`Command failed: ${commandString}`)
  }
}

function main() {
  fs.mkdirSync(backendDir, { recursive: true })

  console.log('[desktop] Building frontend static assets...')
  run('pnpm', ['run', 'build'], { cwd: uiRoot })

  console.log('[desktop] Building Go backend binary...')
  run('go', ['build', '-trimpath', '-o', backendBinary, '.'], {
    cwd: repoRoot,
    env: {
      ...process.env,
      CGO_ENABLED: '0',
    },
  })

  if (process.platform !== 'win32') {
    fs.chmodSync(backendBinary, 0o755)
  }

  console.log(`[desktop] Backend ready: ${backendBinary}`)
}

main()
