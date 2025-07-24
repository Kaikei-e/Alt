{{/*
Expand the name of the chart.
*/}}
{{- define "common-config.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "common-config.fullname" -}}
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
{{- define "common-config.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "common-config.labels" -}}
helm.sh/chart: {{ include "common-config.chart" . }}
{{ include "common-config.selectorLabels" . }}
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
{{- define "common-config.selectorLabels" -}}
app.kubernetes.io/name: {{ include "common-config.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Get namespace name for environment
*/}}
{{- define "common-config.namespaceName" -}}
{{- $env := .env | default "production" -}}
{{- if eq $env "development" -}}
{{- .Values.namespaces.development.name }}
{{- else if eq $env "staging" -}}
{{- .Values.namespaces.staging.name }}
{{- else -}}
{{- .Values.namespaces.production.name }}
{{- end }}
{{- end }}

{{/*
Get service-specific namespace name
*/}}
{{- define "common-config.serviceNamespace" -}}
{{- $serviceType := .serviceType | default "apps" -}}
{{- if eq $serviceType "database" -}}
{{- .Values.namespaces.database.name }}
{{- else if eq $serviceType "search" -}}
{{- .Values.namespaces.search.name }}
{{- else if eq $serviceType "observability" -}}
{{- .Values.namespaces.observability.name }}
{{- else if eq $serviceType "ingress" -}}
{{- .Values.namespaces.ingress.name }}
{{- else if eq $serviceType "auth" -}}
{{- .Values.namespaces.auth.name }}
{{- else -}}
{{- .Values.namespaces.apps.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "common-config.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "common-config.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}