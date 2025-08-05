{{/*
Expand the name of the chart.
*/}}
{{- define "search-indexer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "search-indexer.fullname" -}}
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
{{- define "search-indexer.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "search-indexer.labels" -}}
helm.sh/chart: {{ include "search-indexer.chart" . }}
{{ include "search-indexer.selectorLabels" . }}
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
{{- define "search-indexer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "search-indexer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "search-indexer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "search-indexer.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Standard secret name template
*/}}
{{- define "search-indexer.secretName" -}}
{{- if .Values.envFromSecret.name }}
{{- .Values.envFromSecret.name }}
{{- else }}
{{- printf "%s-secrets" (include "search-indexer.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "search-indexer.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "search-indexer.envFromSecret" -}}
{{- if .Values.envFromSecret }}
{{- range .Values.envFromSecret.keys }}
- name: {{ . }}
  valueFrom:
    secretKeyRef:
      name: {{ include "search-indexer.secretName" $ }}
      key: {{ . }}
{{- end }}
{{- end }}
{{- end }}