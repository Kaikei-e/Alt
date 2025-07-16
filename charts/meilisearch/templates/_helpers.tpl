{{/*
Expand the name of the chart.
*/}}
{{- define "meilisearch.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "meilisearch.fullname" -}}
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
{{- define "meilisearch.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "meilisearch.labels" -}}
helm.sh/chart: {{ include "meilisearch.chart" . }}
{{ include "meilisearch.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: alt
app.kubernetes.io/component: search-engine
{{- end }}

{{/*
Selector labels
*/}}
{{- define "meilisearch.selectorLabels" -}}
app.kubernetes.io/name: {{ include "meilisearch.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "meilisearch.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "meilisearch.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create MeiliSearch service hostname
*/}}
{{- define "meilisearch.serviceHost" -}}
{{- printf "%s.%s.svc.cluster.local" (include "meilisearch.fullname" .) .Release.Namespace }}
{{- end }}

{{/*
Create MeiliSearch HTTP endpoint
*/}}
{{- define "meilisearch.httpEndpoint" -}}
{{- if .Values.ssl.enabled }}
{{- printf "https://%s:%d" (include "meilisearch.serviceHost" .) .Values.service.port }}
{{- else }}
{{- printf "http://%s:%d" (include "meilisearch.serviceHost" .) .Values.service.port }}
{{- end }}
{{- end }}

{{/*
Create MeiliSearch master key secret name
*/}}
{{- define "meilisearch.masterKeySecretName" -}}
{{- if .Values.auth.existingSecret }}
{{- .Values.auth.existingSecret }}
{{- else }}
{{- include "meilisearch.fullname" . }}-master-key
{{- end }}
{{- end }}

{{/*
Create MeiliSearch API keys secret name
*/}}
{{- define "meilisearch.apiKeySecretName" -}}
{{- if .Values.auth.existingApiKeySecret }}
{{- .Values.auth.existingApiKeySecret }}
{{- else }}
{{- include "meilisearch.fullname" . }}-api-keys
{{- end }}
{{- end }}

{{/*
Generate MeiliSearch environment variables
*/}}
{{- define "meilisearch.environment" -}}
- name: MEILI_ENV
  value: {{ .Values.environment | quote }}
- name: MEILI_DB_PATH
  value: {{ .Values.persistence.dataPath | quote }}
- name: MEILI_HTTP_ADDR
  value: "0.0.0.0:{{ .Values.service.port }}"
- name: MEILI_NO_ANALYTICS
  value: {{ .Values.analytics.disabled | quote }}
- name: MEILI_LOG_LEVEL
  value: {{ .Values.logging.level | quote }}
{{- if .Values.auth.masterKeyEnabled }}
- name: MEILI_MASTER_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "meilisearch.masterKeySecretName" . }}
      key: {{ .Values.auth.secretKeys.masterKey }}
{{- end }}
{{- if .Values.search.maxIndexSize }}
- name: MEILI_MAX_INDEX_SIZE
  value: {{ .Values.search.maxIndexSize | quote }}
{{- end }}
{{- if .Values.search.maxTaskQueueSize }}
- name: MEILI_MAX_TASK_QUEUE_SIZE
  value: {{ .Values.search.maxTaskQueueSize | quote }}
{{- end }}
{{- if .Values.search.payloadSizeLimit }}
- name: MEILI_HTTP_PAYLOAD_SIZE_LIMIT
  value: {{ .Values.search.payloadSizeLimit | quote }}
{{- end }}
{{- if .Values.snapshots.enabled }}
- name: MEILI_SNAPSHOT_DIR
  value: {{ .Values.snapshots.path | quote }}
- name: MEILI_SCHEDULE_SNAPSHOT
  value: {{ .Values.snapshots.schedule | quote }}
{{- end }}
{{- if .Values.dumps.enabled }}
- name: MEILI_DUMPS_DIR
  value: {{ .Values.dumps.path | quote }}
{{- end }}
{{- if .Values.ssl.enabled }}
- name: MEILI_SSL_CERT_PATH
  value: {{ .Values.ssl.certPath | quote }}
- name: MEILI_SSL_KEY_PATH
  value: {{ .Values.ssl.keyPath | quote }}
{{- if .Values.ssl.requireAuth }}
- name: MEILI_SSL_AUTH_PATH
  value: {{ .Values.ssl.caPath | quote }}
{{- end }}
{{- end }}
{{- with .Values.extraEnv }}
{{ toYaml . }}
{{- end }}
{{- end }}