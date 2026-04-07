import { spawnSync } from 'node:child_process'

function runCapture(cmd, args, options = {}) {
  const result = spawnSync(cmd, args, {
    encoding: 'utf8',
    ...options,
  })

  if (result.status !== 0) {
    return ''
  }

  return (result.stdout || '').trim()
}

function runGit(repoRoot, args) {
  return runCapture('git', args, { cwd: repoRoot })
}

function resolveVersionFromGit(repoRoot) {
  const exactTag = runGit(repoRoot, [
    'describe',
    '--exact-match',
    '--tags',
    '--match',
    'v*',
    'HEAD',
  ])
  if (exactTag) {
    return exactTag
  }

  const lastTag = runGit(repoRoot, ['describe', '--tags', '--match', 'v*', '--abbrev=0'])
  const commitHashShort = runGit(repoRoot, ['rev-parse', '--short', 'HEAD']) || 'unknown'

  if (!lastTag) {
    return `v0.0.0-${commitHashShort}`
  }

  const commitsAheadRaw = runGit(repoRoot, ['rev-list', '--count', `${lastTag}..HEAD`])
  const commitsAhead = Number.parseInt(commitsAheadRaw, 10)

  if (commitsAhead === 0) {
    return lastTag
  }

  const versionPart = lastTag.replace(/^v/, '').replace(/-.*$/, '')
  const [major = '0', minor = '0', patchRaw = '0'] = versionPart.split('.')
  const patch = Number.parseInt(patchRaw, 10)
  const nextPatch = Number.isNaN(patch) ? 1 : patch + 1

  return `v${major}.${minor}.${nextPatch}-p${commitsAhead}-${commitHashShort}`
}

function normalizeDesktopVersion(version) {
  return version.replace(/^v/, '')
}

export function resolveBuildMetadata(repoRoot) {
  const version = resolveVersionFromGit(repoRoot)
  const commitID = runGit(repoRoot, ['rev-parse', 'HEAD']) || 'unknown'
  const buildDate = new Date().toISOString()
  return {
    version,
    desktopVersion: normalizeDesktopVersion(version),
    commitID,
    buildDate,
  }
}

