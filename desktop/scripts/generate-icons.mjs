import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const desktopRoot = path.resolve(__dirname, '..')
const repoRoot = path.resolve(desktopRoot, '..')
const sourceLogo = path.join(repoRoot, 'ui', 'public', 'logo.svg')
const iconDir = path.join(desktopRoot, 'icons')
const iconSetDir = path.join(iconDir, 'icon.iconset')

function run(cmd, args, options = {}) {
  const result = spawnSync(cmd, args, {
    stdio: 'inherit',
    ...options,
  })
  if (result.status !== 0) {
    throw new Error(`Command failed: ${cmd} ${args.join(' ')}`)
  }
}

function main() {
  if (!fs.existsSync(sourceLogo)) {
    throw new Error(`Logo not found: ${sourceLogo}`)
  }

  fs.mkdirSync(iconSetDir, { recursive: true })
  fs.copyFileSync(sourceLogo, path.join(iconDir, 'logo.svg'))

  const icon1024 = path.join(iconDir, 'icon-1024.png')
  run('sips', ['-s', 'format', 'png', sourceLogo, '--out', icon1024])
  fs.copyFileSync(icon1024, path.join(iconDir, 'icon.png'))

  const sizes = [16, 32, 128, 256, 512]
  for (const size of sizes) {
    run('sips', [
      '-z',
      String(size),
      String(size),
      icon1024,
      '--out',
      path.join(iconSetDir, `icon_${size}x${size}.png`),
    ])
    run('sips', [
      '-z',
      String(size * 2),
      String(size * 2),
      icon1024,
      '--out',
      path.join(iconSetDir, `icon_${size}x${size}@2x.png`),
    ])
  }

  run('iconutil', ['-c', 'icns', iconSetDir, '-o', path.join(iconDir, 'icon.icns')])
  run('magick', [
    path.join(iconSetDir, 'icon_16x16.png'),
    path.join(iconSetDir, 'icon_32x32.png'),
    path.join(iconSetDir, 'icon_128x128.png'),
    path.join(iconSetDir, 'icon_256x256.png'),
    path.join(iconSetDir, 'icon_512x512.png'),
    path.join(iconDir, 'icon.ico'),
  ])

  console.log('[desktop] Icons generated in desktop/icons')
}

main()
