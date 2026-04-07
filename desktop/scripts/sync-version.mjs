import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { resolveBuildMetadata } from './build-meta.mjs'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const desktopRoot = path.resolve(__dirname, '..')
const repoRoot = path.resolve(desktopRoot, '..')
const desktopPackageJSON = path.join(desktopRoot, 'package.json')

function main() {
  const meta = resolveBuildMetadata(repoRoot)
  const packageRaw = fs.readFileSync(desktopPackageJSON, 'utf8')
  const packageJSON = JSON.parse(packageRaw)

  if (packageJSON.version === meta.desktopVersion) {
    console.log(`[desktop] version already synced: ${meta.desktopVersion}`)
    return
  }

  packageJSON.version = meta.desktopVersion
  fs.writeFileSync(desktopPackageJSON, `${JSON.stringify(packageJSON, null, 2)}\n`)
  console.log(`[desktop] synced desktop package version: ${meta.desktopVersion}`)
}

main()

