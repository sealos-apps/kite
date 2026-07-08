import * as yaml from 'js-yaml'

type DumpOptions = Parameters<typeof yaml.dump>[1]

const KUBERNETES_YAML_DUMP_OPTIONS: DumpOptions = {
  indent: 2,
  lineWidth: -1,
  noRefs: true,
}

export function dumpKubernetesYaml(
  value: unknown,
  options: DumpOptions = {}
): string {
  return yaml.dump(value, {
    ...KUBERNETES_YAML_DUMP_OPTIONS,
    ...options,
  })
}
