import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const terminalComponents = [
  { name: 'resource terminal', file: './terminal.tsx' },
  { name: 'floating terminal', file: './terminal-content.tsx' },
]

for (const component of terminalComponents) {
  test(`${component.name} relies on protocol-level WebSocket heartbeats`, () => {
    const source = readFileSync(
      new URL(component.file, import.meta.url),
      'utf8'
    )

    assert.doesNotMatch(source, /type\s*:\s*['"]ping['"]/, component.file)
    assert.doesNotMatch(source, /case\s+['"]pong['"]/, component.file)
  })
}
