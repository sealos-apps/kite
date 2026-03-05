{{/*
Expand the name of the chart.
*/}}
{{- define "kite.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kite.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kite.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kite.labels" -}}
helm.sh/chart: {{ include "kite.chart" . }}
{{ include "kite.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kite.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kite.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kite.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kite.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "kite.postgresNativeEnabled" -}}
{{- $dbType := default "sqlite" .Values.db.type -}}
{{- $postgres := default (dict) .Values.db.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- $enabled := true -}}
{{- if hasKey $native "enabled" -}}
{{- $enabled = $native.enabled -}}
{{- end -}}
{{- if and (eq $dbType "postgres") $enabled -}}true{{- else -}}false{{- end -}}
{{- end }}

{{- define "kite.postgresClusterName" -}}
{{- $postgres := default (dict) .Values.db.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- $clusterName := (default "" $native.clusterName) | trim -}}
{{- if $clusterName -}}
{{- $clusterName -}}
{{- else -}}
{{- printf "%s-pg" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end }}

{{- define "kite.postgresCredentialSecretName" -}}
{{- $postgres := default (dict) .Values.db.postgres -}}
{{- $native := default (dict) $postgres.native -}}
{{- $secretName := (default "" $native.credentialSecretName) | trim -}}
{{- if $secretName -}}
{{- $secretName -}}
{{- else -}}
{{- printf "%s-conn-credential" (include "kite.postgresClusterName" .) -}}
{{- end -}}
{{- end }}


{{- define "kite.secret" -}}
{{- if .Values.secret.existingSecret }}
{{- .Values.secret.existingSecret }}
{{- else }}
{{- printf "%s-secret" (include "kite.fullname" .) }}
{{- end }}
{{- end }}
