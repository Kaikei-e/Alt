{{/*
Expand the name of the chart.
*/}}
{{- define "clickhouse.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "clickhouse.fullname" -}}
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
{{- define "clickhouse.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "clickhouse.labels" -}}
helm.sh/chart: {{ include "clickhouse.chart" . }}
{{ include "clickhouse.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: alt
app.kubernetes.io/component: analytics-database
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "clickhouse.selectorLabels" -}}
app.kubernetes.io/name: {{ include "clickhouse.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "clickhouse.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "clickhouse.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create ClickHouse HTTP connection string
*/}}
{{- define "clickhouse.httpConnectionString" -}}
{{- if .Values.ssl.enabled }}
{{- printf "https://%s:%s@%s:%d/%s" .Values.auth.username .Values.auth.password (include "clickhouse.fullname" .) (.Values.service.httpPort | int) .Values.auth.database }}
{{- else }}
{{- printf "http://%s:%s@%s:%d/%s" .Values.auth.username .Values.auth.password (include "clickhouse.fullname" .) (.Values.service.httpPort | int) .Values.auth.database }}
{{- end }}
{{- end }}

{{/*
Create ClickHouse TCP connection string
*/}}
{{- define "clickhouse.tcpConnectionString" -}}
{{- printf "clickhouse://%s:%s@%s:%d/%s" .Values.auth.username .Values.auth.password (include "clickhouse.fullname" .) (.Values.service.tcpPort | int) .Values.auth.database }}
{{- end }}

{{/*
Create ClickHouse service hostname
*/}}
{{- define "clickhouse.serviceHost" -}}
{{- printf "%s.%s.svc.cluster.local" (include "clickhouse.fullname" .) .Release.Namespace }}
{{- end }}

{{/*
Create ClickHouse HTTP service hostname
*/}}
{{- define "clickhouse.httpServiceHost" -}}
{{- printf "%s:%d" (include "clickhouse.serviceHost" .) (.Values.service.httpPort | int) }}
{{- end }}

{{/*
Create ClickHouse TCP service hostname
*/}}
{{- define "clickhouse.tcpServiceHost" -}}
{{- printf "%s:%d" (include "clickhouse.serviceHost" .) (.Values.service.tcpPort | int) }}
{{- end }}

{{/*
Create data directory path
*/}}
{{- define "clickhouse.dataDir" -}}
{{- printf "/var/lib/clickhouse" }}
{{- end }}

{{/*
Create log directory path
*/}}
{{- define "clickhouse.logDir" -}}
{{- printf "/var/log/clickhouse-server" }}
{{- end }}