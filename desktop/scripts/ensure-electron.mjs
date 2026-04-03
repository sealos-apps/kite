import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)

function resolveElectronDir() {
  const electronPackageJson = require.resolve('electron/package.json')
  return path.dirname(electronPackageJson)
}

function isElectronInstalled(electronDir) {
  const pathFile = path.join(electronDir, 'path.txt')
  if (!fs.existsSync(pathFile)) {
    return false
  }

  const relativeExecutable = fs.readFileSync(pathFile, 'utf8').trim()
  if (!relativeExecutable) {
    return false
  }

  const executablePath = path.join(electronDir, 'dist', relativeExecutable)
  return fs.existsSync(executablePath)
}

function getDefaultExecutableRelativePath() {
  if (process.platform === 'darwin') {
    return 'Electron.app/Contents/MacOS/Electron'
  }
  if (process.platform === 'win32') {
    return 'electron.exe'
  }
  return 'electron'
}

function resolveDistExecutableRelativePath(electronDir) {
  const relative = getDefaultExecutableRelativePath()
  const executablePath = path.join(electronDir, 'dist', relative)
  if (!fs.existsSync(executablePath)) {
    return null
  }
  return relative
}

function writeElectronPathFile(electronDir, relativeExecutablePath) {
  const pathFile = path.join(electronDir, 'path.txt')
  fs.writeFileSync(pathFile, relativeExecutablePath)
}

function installElectronBinary(electronDir) {
  const result = spawnSync(process.execPath, ['install.js'], {
    cwd: electronDir,
    stdio: 'inherit',
    env: process.env,
  })
  return result.status === 0
}

function main() {
  const electronDir = resolveElectronDir()
  if (isElectronInstalled(electronDir)) {
    console.log('[desktop] Electron binary is ready')
    return
  }

  // Recover from partial installs where dist exists but path.txt is missing.
  const existingRelativePath = resolveDistExecutableRelativePath(electronDir)
  if (existingRelativePath) {
    writeElectronPathFile(electronDir, existingRelativePath)
    if (isElectronInstalled(electronDir)) {
      console.log('[desktop] Electron path file repaired')
      return
    }
  }

  console.log('[desktop] Electron binary missing, installing...')
  const installed = installElectronBinary(electronDir)

  // install.js can fail when files already exist; repair path file if dist is present.
  const repairedRelativePath = resolveDistExecutableRelativePath(electronDir)
  if (repairedRelativePath) {
    writeElectronPathFile(electronDir, repairedRelativePath)
  }

  if (!isElectronInstalled(electronDir)) {
    if (!installed) {
      throw new Error('Failed to install Electron binary')
    }
    throw new Error('Electron binary verification failed after install')
  }
  console.log('[desktop] Electron binary installed')
}

main()
