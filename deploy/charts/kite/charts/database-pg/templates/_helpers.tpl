{{- define "database-pg.externalDSN" -}}
{{- $dsn := (default "" .Values.dsn) | trim -}}
{{- $dns := "" -}}
{{- if hasKey .Values "dns" -}}
{{- $dns = (default "" .Values.dns) | trim -}}
{{- end -}}
{{- if $dsn -}}
{{- $dsn -}}
{{- else -}}
{{- $dns -}}
{{- end -}}
{{- end }}

{{- define "database-pg.nativeEnabled" -}}
{{- $dbType := default "sqlite" .Values.type -}}
{{- $postgres := default (dict) .Values.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- $enabled := true -}}
{{- $externalDSN := include "database-pg.externalDSN" . | trim -}}
{{- if hasKey $native "enabled" -}}
{{- $enabled = $native.enabled -}}
{{- end -}}
{{- if and (eq $dbType "postgres") $enabled (eq $externalDSN "") -}}true{{- else -}}false{{- end -}}
{{- end }}

{{- define "database-pg.clusterName" -}}
{{- $postgres := default (dict) .Values.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- $clusterName := (default "" $native.clusterName) | trim -}}
{{- if $clusterName -}}
{{- $clusterName -}}
{{- else -}}
{{- printf "%s-pg" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end }}

{{- define "database-pg.version" -}}
{{- $postgres := default (dict) .Values.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- default "postgresql-14.8.0" $native.version -}}
{{- end }}

{{- define "database-pg.terminationPolicy" -}}
{{- $postgres := default (dict) .Values.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- default "Delete" $native.terminationPolicy -}}
{{- end }}
