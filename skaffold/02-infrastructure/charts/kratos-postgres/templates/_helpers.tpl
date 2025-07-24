{{/*
Expand the name of the chart.
*/}}
{{- define "kratos-postgres.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kratos-postgres.fullname" -}}
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
{{- define "kratos-postgres.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kratos-postgres.labels" -}}
helm.sh/chart: {{ include "kratos-postgres.chart" . }}
{{ include "kratos-postgres.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: alt
app.kubernetes.io/component: identity-database
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kratos-postgres.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kratos-postgres.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kratos-postgres.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kratos-postgres.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create database connection string
*/}}
{{- define "kratos-postgres.connectionString" -}}
{{- printf "postgresql://%s:%s@%s:%d/%s" .Values.auth.username .Values.auth.password (include "kratos-postgres.fullname" .) .Values.service.port .Values.auth.database }}
{{- end }}

{{/*
Create SSL connection string
*/}}
{{- define "kratos-postgres.sslConnectionString" -}}
{{- if .Values.ssl.enabled }}
{{- printf "postgresql://%s:%s@%s:%d/%s?sslmode=%s&sslcert=%s&sslkey=%s&sslrootcert=%s" .Values.auth.username .Values.auth.password (include "kratos-postgres.fullname" .) .Values.service.port .Values.auth.database .Values.ssl.mode .Values.ssl.certPath .Values.ssl.keyPath .Values.ssl.caPath }}
{{- else }}
{{- include "kratos-postgres.connectionString" . }}
{{- end }}
{{- end }}