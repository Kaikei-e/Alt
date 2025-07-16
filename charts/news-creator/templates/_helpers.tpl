{{/*
Expand the name of the chart.
*/}}
{{- define "news-creator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "news-creator.fullname" -}}
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
{{- define "news-creator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "news-creator.labels" -}}
helm.sh/chart: {{ include "news-creator.chart" . }}
{{ include "news-creator.selectorLabels" . }}
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
{{- define "news-creator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "news-creator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "news-creator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "news-creator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "news-creator.envFromSecret" -}}
{{- if .Values.envFromSecret }}
{{- range .Values.envFromSecret.keys }}
- name: {{ . }}
  valueFrom:
    secretKeyRef:
      name: {{ $.Values.envFromSecret.name }}
      key: {{ . }}
{{- end }}
{{- end }}
{{- end }}