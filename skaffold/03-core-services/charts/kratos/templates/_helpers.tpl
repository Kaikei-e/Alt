{{/*
Expand the name of the chart.
*/}}
{{- define "kratos.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kratos.fullname" -}}
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
{{- define "kratos.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kratos.labels" -}}
helm.sh/chart: {{ include "kratos.chart" . }}
{{ include "kratos.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: alt
app.kubernetes.io/component: identity
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kratos.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kratos.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kratos.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kratos.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Generate database DSN
*/}}
{{- define "kratos.databaseDSN" -}}
postgres://{{ .Values.database.username }}:$(POSTGRES_PASSWORD)@{{ .Values.database.host }}:{{ .Values.database.port }}/{{ .Values.database.database }}?sslmode={{ .Values.database.ssl_mode }}&max_conns={{ .Values.database.max_conns }}&max_idle_conns={{ .Values.database.max_idle_conns }}
{{- end }}