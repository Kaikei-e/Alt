{{/*
Expand the name of the chart.
*/}}
{{- define "common-secrets.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "common-secrets.fullname" -}}
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
{{- define "common-secrets.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "common-secrets.labels" -}}
helm.sh/chart: {{ include "common-secrets.chart" . }}
{{ include "common-secrets.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "common-secrets.selectorLabels" -}}
app.kubernetes.io/name: {{ include "common-secrets.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "common-secrets.serviceAccountName" -}}
{{- if or .Values.serviceAccount.create .Values.externalSecrets.createServiceAccount }}
{{- default (include "common-secrets.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Generate database URL for PostgreSQL
*/}}
{{- define "common-secrets.postgresUrl" -}}
{{- $user := .user | default "postgres" -}}
{{- $host := .host | default "postgres" -}}
{{- $port := .port | default 5432 -}}
{{- $database := .database | default "postgres" -}}
{{- $sslmode := .sslmode | default "require" -}}
postgresql://{{ $user }}:{{ .password }}@{{ $host }}:{{ $port }}/{{ $database }}?sslmode={{ $sslmode }}
{{- end }}