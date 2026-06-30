# Chart Values

本文档描述了 Kite Helm Chart 的所有可用配置选项。

## 基础配置

| 参数               | 描述                                               | 默认值                |
| ------------------ | -------------------------------------------------- | --------------------- |
| `replicaCount`     | 副本数量                                           | `1`                   |
| `image.repository` | 容器镜像仓库                                       | `crpi-7jr40k6elhldekqp.cn-hangzhou.personal.cr.aliyuncs.com/mlhiter/kite` |
| `image.pullPolicy` | 镜像拉取策略                                       | `IfNotPresent`        |
| `image.tag`        | 镜像标签。如果设置，将覆盖 chart 的 `appVersion`。 | `"1.0.8"`             |
| `imagePullSecrets` | 私有镜像仓库的拉取密钥                             | `[]`                  |
| `nameOverride`     | 覆盖 chart 名称                                    | `""`                  |
| `fullnameOverride` | 覆盖完整名称                                       | `""`                  |
| `debug`            | 启用调试模式                                       | `false`               |
| `basePath`         | 应用的基础路径，详见安装文档中的说明。     | `""`                 |

## 认证与安全

| 参数                   | 描述                                                       | 默认值                                               |
| ---------------------- | ---------------------------------------------------------- | ---------------------------------------------------- |
| `anonymousUserEnabled` | 启用匿名用户访问，拥有完全管理员权限。生产环境请谨慎使用。 | `false`                                              |
| `jwtSecret`            | 用于签名 JWT 令牌的密钥。生产环境请修改此值。              | `"kite-default-jwt-secret-key-change-in-production"` |
| `encryptKey`           | 用于加密敏感数据的密钥。生产环境请修改此值。               | `"kite-default-encryption-key-change-in-production"` |
| `host`                 | 应用程序的主机名                                           | `""`                                                 |
| `cloudDomain`          | Sealos 域名，用于渲染 ingress/app 主机名                   | `"127.0.0.1.nip.io"`                                |
| `cloudPort`            | HTTPS 模式下渲染 Sealos App 外部 URL 使用的端口             | `"443"`                                             |
| `httpPort`             | `disableHttps=true` 时渲染 HTTP 外部 URL 使用的端口          | `"80"`                                              |
| `disableHttps`         | 以 HTTP/无 TLS 模式渲染 Sealos App URL 和 Ingress            | `false`                                             |
| `certSecretName`       | HTTPS 模式下 Ingress TLS 使用的 Secret 名称                 | `"wildcard-cert"`                                   |
| `platform.tlsRejectUnauthorized` | 注入环境变量 `NODE_TLS_REJECT_UNAUTHORIZED` 的值    | `"1"`                                               |
| `sealos.jwtSecret`     | 注入环境变量 `SEALOS_JWT_SECRET` 的值                      | `""`                                                 |

## 数据库配置

| 参数            | 描述                                                                  | 默认值     |
| --------------- | --------------------------------------------------------------------- | ---------- |
| `db.type`       | 数据库类型：`sqlite`、`postgres`、`mysql`                             | `postgres` |
| `db.dsn`        | MySQL/Postgres 的完整 DSN 字符串。关闭内置 Postgres 且使用外部库时必需 | `""`       |
| `db.autoCreate` | Kite 是否在运行表结构迁移前自动创建目标 MySQL/Postgres database       | `true`     |
| `db.postgres.native.kubeblocksVersion` | 部署入口读取的 Sealos 全局 KubeBlocks 版本标记        | `"0.8.2"` |

开启 `db.autoCreate` 时，配置的数据库账号需要有创建 database 的权限。Kite 只负责创建目标 database，表结构仍由应用正常迁移流程创建。

### SQLite 配置

| 参数                                      | 描述                                  | 默认值              |
| ----------------------------------------- | ------------------------------------- | ------------------- |
| `db.sqlite.persistence.pvc.enabled`       | 是否创建 PVC 来存储 sqlite 数据库文件 | `false`             |
| `db.sqlite.persistence.pvc.existingClaim` | 使用现有的 PVC                        | `""`                |
| `db.sqlite.persistence.pvc.storageClass`  | PVC 的 StorageClass（可选）           | `""`                |
| `db.sqlite.persistence.pvc.accessModes`   | PVC 的访问模式                        | `["ReadWriteOnce"]` |
| `db.sqlite.persistence.pvc.size`          | PVC 请求的存储大小                    | `1Gi`               |
| `db.sqlite.persistence.hostPath.enabled`  | 是否使用 hostPath 存储                | `false`             |
| `db.sqlite.persistence.hostPath.path`     | hostPath 路径                         | `/path/to/host/dir` |
| `db.sqlite.persistence.hostPath.type`     | hostPath 类型                         | `DirectoryOrCreate` |
| `db.sqlite.persistence.mountPath`         | 容器内的挂载路径                      | `/data`             |
| `db.sqlite.persistence.filename`          | 挂载路径内的 sqlite 文件名            | `kite.db`           |

## 环境变量

| 参数        | 描述               | 默认值 |
| ----------- | ------------------ | ------ |
| `extraEnvs` | 额外的环境变量列表 | `[]`   |

## Helm Chart Catalog 配置

| 参数                                           | 描述                                           | 默认值    |
| ---------------------------------------------- | ---------------------------------------------- | --------- |
| `helmCatalog.artifactHub.enabled`              | 是否启用 Artifact Hub Chart 搜索/详情代理 API | `true`    |
| `helmCatalog.oci.base`                         | 用于发现离线 Helm OCI Chart 的 `oci://` registry 前缀 | `""` |
| `helmCatalog.oci.repositoryName`               | Kite 页面中展示的 OCI Chart 仓库名称          | `offline` |
| `helmCatalog.oci.discoveryPageSize`            | 列出 registry repositories/tags 时使用的分页大小 | `100` |
| `helmCatalog.oci.discoveryMaxRepositories`     | 查找配置前缀时最多检查的 registry repository 数量 | `1000` |
| `helmCatalog.oci.discoveryMaxTagsPerRepository` | 每个 repository 最多扫描的 tag 数量           | `200`     |
| `helmCatalog.oci.plainHTTP`                    | 是否使用 HTTP 访问 OCI registry API 和 Chart 包 | `false` |
| `helmCatalog.oci.insecureSkipTLSVerify`        | 是否跳过私有 registry/token endpoint 的 TLS 校验 | `false` |
| `helmCatalog.oci.caFile`                       | 挂载到 Kite 容器内的私有 registry CA bundle 路径 | `""` |
| `helmCatalog.oci.username`                     | Kite 访问 OCI registry 列表和 Chart 包时使用的用户名 | `""` |
| `helmCatalog.oci.password`                     | 写入 Kite Secret 的 OCI registry 密码；生产安装优先使用 `passwordSecretName` | `""` |
| `helmCatalog.oci.passwordSecretName`           | 保存 OCI registry 密码的已有 Secret           | `""`      |
| `helmCatalog.oci.passwordSecretKey`            | `passwordSecretName` 中作为 `KITE_HELM_OCI_REGISTRY_PASSWORD` 使用的 key | `KITE_HELM_OCI_REGISTRY_PASSWORD` |
| `helmCatalog.offlineImages.enabled`            | 为 OCI catalog 安装/升级注入离线容器镜像默认值 | `false` |
| `helmCatalog.offlineImages.registry`           | 离线 OCI Chart 渲染 workload 镜像时应使用的 registry host | `""` |
| `helmCatalog.offlineImages.enforce`            | 当渲染出的 workload 镜像仍指向离线 registry 之外时阻止安装/升级 | `true` |

离线部署可以关闭 Artifact Hub，并让 Kite 只扫描一个受控 OCI registry
前缀：

```yaml
helmCatalog:
  artifactHub:
    enabled: false
  oci:
    base: oci://registry.internal/kite-helm
    repositoryName: offline
    plainHTTP: true
    insecureSkipTLSVerify: true
    username: admin
    passwordSecretName: registry-credentials
    passwordSecretKey: KITE_HELM_OCI_REGISTRY_PASSWORD
  offlineImages:
    enabled: true
    registry: registry.internal
    enforce: true
```

如果 registry 中存在 `kite-helm/demo-chart` 的 `0.1.0` 和 `0.2.0`
tag，Kite 会解析为
`oci://registry.internal/kite-helm/demo-chart:0.2.0`，用于浏览、安装、
升级和自动升级。`base` 必须是带 repository 前缀的 `oci://` URL，不能包含
凭据、查询参数、fragment、tag 或 digest。私有 registry 在生产安装中推荐使用
`helmCatalog.oci.username` 加 `helmCatalog.oci.passwordSecretName`；简易安装
仍可使用 `helmCatalog.oci.password`，它会写入 chart 管理的 Secret。

`helmCatalog.offlineImages` 与 OCI Chart 发现是两件事。Helm OCI Chart
artifact 放在配置的 Chart 前缀下，例如
`oci://registry.internal/kite-helm/nginx:25.0.12`；容器镜像放在同一个
registry host 的原始仓库路径下，例如
`registry.internal/bitnami/nginx:latest`。对 OCI catalog Chart，Kite 会在
用户未显式配置时注入 `global.imageRegistry=<registry>` 和
`global.security.allowInsecureImages=true`，渲染 release，并在 workload 镜像
仍指向外部 registry 时阻止写入。Kite 会把 Chart 来源记录在已安装 release
中，所以从 OCI catalog 安装的 release 后续即使客户端沿用当前 Chart、没有
再次传 `source`，升级时也会继续应用离线镜像策略。Kite 安装时不会复制容器
镜像；需要提前使用 `scripts/mirror-helm-chart-images.sh` 或等价 registry
同步流程完成镜像同步。

## Sealos App 配置

| 参数          | 描述                                  | 默认值 |
| ------------- | ------------------------------------- | ------ |
| `app.enabled` | 是否创建 `app.sealos.io/v1 App`      | `false` |

App 元数据在模板中固定：
- namespace: `app-system`
- name: `kite`
- type/displayType: `iframe` / `normal`
- icon/url：根据 `disableHttps`、`cloudPort` 和 `httpPort` 渲染。

## 服务账户配置

| 参数                         | 描述                        | 默认值 |
| ---------------------------- | --------------------------- | ------ |
| `serviceAccount.create`      | 是否创建服务账户            | `true` |
| `serviceAccount.automount`   | 自动挂载服务账户的 API 凭据 | `true` |
| `serviceAccount.annotations` | 服务账户的注解              | `{}`   |
| `serviceAccount.name`        | 使用的服务账户名称          | `""`   |

## RBAC 配置

| 参数          | 描述               | 默认值     |
| ------------- | ------------------ | ---------- |
| `rbac.create` | 是否创建 RBAC 资源 | `true`     |
| `rbac.rules`  | RBAC 规则列表      | 见下方示例 |

### RBAC 规则示例

```yaml
rbac:
  rules:
    - apiGroups: ["*"]
      resources: ["*"]
      verbs: ["*"]
    - nonResourceURLs: ["*"]
      verbs: ["*"]
```

## Pod 配置

| 参数                 | 描述                   | 默认值 |
| -------------------- | ---------------------- | ------ |
| `podAnnotations`     | Pod 的 Kubernetes 注解 | `{}`   |
| `podLabels`          | Pod 的 Kubernetes 标签 | `{}`   |
| `podSecurityContext` | Pod 安全上下文         | `{}`   |
| `securityContext`    | 容器安全上下文         | `{}`   |

## 服务配置

| 参数           | 描述     | 默认值      |
| -------------- | -------- | ----------- |
| `service.type` | 服务类型 | `ClusterIP` |
| `service.port` | 服务端口 | `8080`      |

## Ingress 配置

| 参数                  | 描述             | 默认值     |
| --------------------- | ---------------- | ---------- |
| `ingress.enabled`     | 是否启用 Ingress | `true`     |

Ingress 行为在模板中固定：
- host: `kite.<cloudDomain>`
- className: `nginx`
- path/pathType: `/` / `Prefix`
- TLS：仅在 `disableHttps=false` 时渲染，secret 名称来自 `certSecretName`
- annotations：
  - `nginx.ingress.kubernetes.io/proxy-read-timeout: '3600'`
  - `nginx.ingress.kubernetes.io/proxy-send-timeout: '3600'`

### Ingress 配置示例

```yaml
ingress:
  enabled: true
cloudDomain: 192.168.10.70.nip.io
```

## 资源限制

| 参数        | 描述               | 默认值 |
| ----------- | ------------------ | ------ |
| `resources` | 容器资源限制和请求 | `{}`   |

### 资源限制示例

```yaml
resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

## 健康检查

| 参数             | 描述         | 默认值     |
| ---------------- | ------------ | ---------- |
| `livenessProbe`  | 存活探针配置 | 见下方示例 |
| `readinessProbe` | 就绪探针配置 | 见下方示例 |

### 健康检查示例

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /healthz
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
```

## 存储配置

| 参数           | 描述             | 默认值 |
| -------------- | ---------------- | ------ |
| `volumes`      | 额外的卷配置     | `[]`   |
| `volumeMounts` | 额外的卷挂载配置 | `[]`   |

## 调度配置

| 参数           | 描述       | 默认值 |
| -------------- | ---------- | ------ |
| `nodeSelector` | 节点选择器 | `{}`   |
| `tolerations`  | 容忍度配置 | `[]`   |
| `affinity`     | 亲和性配置 | `{}`   |
