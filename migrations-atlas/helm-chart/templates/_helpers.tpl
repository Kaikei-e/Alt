{{/*
Expand the name of the chart.
*/}}
{{- define "alt-atlas-migrations.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "alt-atlas-migrations.fullname" -}}
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
{{- define "alt-atlas-migrations.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "alt-atlas-migrations.labels" -}}
helm.sh/chart: {{ include "alt-atlas-migrations.chart" . }}
{{ include "alt-atlas-migrations.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: database-migration
app.kubernetes.io/part-of: alt-rss-reader
{{- end }}

{{/*
Selector labels
*/}}
{{- define "alt-atlas-migrations.selectorLabels" -}}
app.kubernetes.io/name: {{ include "alt-atlas-migrations.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "alt-atlas-migrations.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "alt-atlas-migrations.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create database URL from components
*/}}
{{- define "alt-atlas-migrations.databaseURL" -}}
{{- $host := .Values.database.host }}
{{- $port := .Values.database.port }}
{{- $name := .Values.database.name }}
{{- $sslMode := .Values.database.sslMode }}
{{- printf "postgres://$(DB_USERNAME):$(DB_PASSWORD)@%s:%d/%s?sslmode=%s" $host $port $name $sslMode }}
{{- end }}

{{/*
Environment-specific overrides
*/}}
{{- define "alt-atlas-migrations.environmentConfig" -}}
{{- $env := .Values.environment | default "production" }}
{{- if hasKey .Values.environments $env }}
{{- $envConfig := index .Values.environments $env }}
{{- toYaml $envConfig }}
{{- end }}
{{- end }}

{{/*
Migration job name with command suffix
*/}}
{{- define "alt-atlas-migrations.jobName" -}}
{{- printf "%s-%s" (include "alt-atlas-migrations.fullname" .) .Values.migration.command }}
{{- end }}

{{/*
Create database connection secret name
*/}}
{{- define "alt-atlas-migrations.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else }}
{{- .Values.secrets.name | default (printf "%s-db-secret" (include "alt-atlas-migrations.fullname" .)) }}
{{- end }}
{{- end }}