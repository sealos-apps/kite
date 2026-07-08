import assert from 'node:assert/strict'
import { test } from 'node:test'

import * as yaml from 'js-yaml'

import { dumpKubernetesYaml } from './yaml.ts'

test('dumpKubernetesYaml does not fold multiline scripts', () => {
  const script = [
    'cat <<EOF',
    '{"script":"echo hello && echo world && echo this is a very very very very very long command"}',
    'EOF',
    '',
  ].join('\n')

  const dumped = dumpKubernetesYaml({ data: { script } })
  const parsed = yaml.load(dumped) as { data: { script: string } }

  assert.equal(/^ {2}script: >/m.test(dumped), false)
  assert.equal(parsed.data.script, script)
})
