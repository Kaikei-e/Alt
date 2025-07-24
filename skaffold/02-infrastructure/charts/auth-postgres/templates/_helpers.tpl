{{/*
Expand the name of the chart.
*/}}
{{- define "auth-postgres.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "auth-postgres.fullname" -}}
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
{{- define "auth-postgres.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "auth-postgres.labels" -}}
helm.sh/chart: {{ include "auth-postgres.chart" . }}
{{ include "auth-postgres.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: alt
app.kubernetes.io/component: auth-database
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "auth-postgres.selectorLabels" -}}
app.kubernetes.io/name: {{ include "auth-postgres.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "auth-postgres.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "auth-postgres.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create database connection string
*/}}
{{- define "auth-postgres.connectionString" -}}
{{- printf "postgresql://%s:%s@%s:%d/%s" .Values.auth.username .Values.auth.password (include "auth-postgres.fullname" .) .Values.service.port .Values.auth.database }}
{{- end }}

{{/*
Create SSL connection string
*/}}
{{- define "auth-postgres.sslConnectionString" -}}
{{- if .Values.ssl.enabled }}
{{- printf "postgresql://%s:%s@%s:%d/%s?sslmode=%s&sslcert=%s&sslkey=%s&sslrootcert=%s" .Values.auth.username .Values.auth.password (include "auth-postgres.fullname" .) .Values.service.port .Values.auth.database .Values.ssl.mode .Values.ssl.certPath .Values.ssl.keyPath .Values.ssl.caPath }}
{{- else }}
{{- include "auth-postgres.connectionString" . }}
{{- end }}
{{- end }}

{{/*
Create auth database service hostname
*/}}
{{- define "auth-postgres.serviceHost" -}}
{{- printf "%s.%s.svc.cluster.local" (include "auth-postgres.fullname" .) .Release.Namespace }}
{{- end }}