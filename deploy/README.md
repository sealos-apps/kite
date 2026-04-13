# Kite Deploy

`kite/deploy/` 提供源码仓库内的 Sealos 打包部署入口，结构参考 Sealos 标准化 deploy 目录。

## 目录结构

```text
deploy/
├── charts/
│   └── kite/
│       ├── Chart.yaml
│       ├── values.yaml
│       ├── kite-values.yaml
│       └── templates/
├── Kubefile
├── kite-entrypoint.sh
└── install-legacy.yaml
```

## 配置加载顺序

Sealos 部署时，入口脚本会按以下优先级加载配置（后者覆盖前者）：

1. `./charts/kite/values.yaml`
2. `/root/.sealos/cloud/values/core/kite-values.yaml`
3. 入口脚本自动注入的 Helm 参数
4. `HELM_OPTS`

其中：

- `values.yaml` 保留 chart 的完整默认值，保证公开 Helm Chart 默认安装兼容
- `kite-values.yaml` 是 Sealos 用户自定义模板，首次部署时会自动复制到 `/root/.sealos/cloud/values/core/`

## 自动注入项

`kite-entrypoint.sh` 会自动处理以下配置：

- `cloudDomain`：来自 `sealos-system/sealos-config`
- `sealos.jwtSecret`：来自 `sealos-system/sealos-config`
- `jwtSecret`：优先复用已有 Secret，不存在时自动生成
- `encryptKey`：优先复用已有 Secret，不存在时自动生成

## 外部数据库模式

如果通过 `db.dsn` 提供外部 PostgreSQL/MySQL 连接，chart 会进入外部数据库模式：

- 自动停止渲染原生 Kubeblocks PostgreSQL 资源
- Deployment 不再引用 Kubeblocks 生成的 credential Secret
- 改为通过应用 Secret 直接注入 `DB_TYPE` 和 `DB_DSN`

兼容性上，也接受 `db.dns` 作为 `db.dsn` 的别名输入，但文档和示例仍推荐使用 `db.dsn`

## 常用环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `RELEASE_NAME` | `kite` | Helm Release 名称 |
| `RELEASE_NAMESPACE` | `kite-system` | 安装命名空间 |
| `NAMESPACE` | - | 兼容旧变量，未设置 `RELEASE_NAMESPACE` 时生效 |
| `CHART_PATH` | `./charts/kite` | Chart 路径 |
| `HELM_OPTS` | `""` | 额外 Helm 参数，优先级最高 |
| `ENABLE_APP` | `""` | 显式覆盖 `app.enabled`，未设置时沿用 `kite-values.yaml` |
| `STRICT_SECRET_REUSE` | `true` | 升级已有 release 时是否强制复用旧 Secret |
| `USER_VALUES_PATH` | `/root/.sealos/cloud/values/core/kite-values.yaml` | 用户 values 文件路径 |

## 使用示例

### Sealos 部署

```bash
sealos run <kite-cluster-image:tag>
```

### 修改资源和副本数

```bash
sealos run <kite-cluster-image:tag> \
  --env HELM_OPTS="--set replicaCount=2 --set resources.limits.cpu=1000m --set resources.limits.memory=1Gi"
```

### 禁用 App CR

```bash
sealos run <kite-cluster-image:tag> \
  --env ENABLE_APP=false
```

### 使用已有 Secret

```bash
sealos run <kite-cluster-image:tag> \
  --env HELM_OPTS="--set secret.create=false --set-string secret.existingSecret=my-kite-secret"
```

### 使用外部 PostgreSQL

```bash
sealos run <kite-cluster-image:tag> \
  --env HELM_OPTS="--set db.type=postgres --set-string db.dsn=host=postgres.example.svc port=5432 user=kite password=secret dbname=kite sslmode=disable"
```

## 入口说明

- `kite-entrypoint.sh` 是 Sealos 打包镜像内的唯一入口
- `install-legacy.yaml` 仅用于快速 legacy 安装，不参与 Sealos values 叠加逻辑
