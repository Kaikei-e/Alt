{{/*
Expand the name of the chart.
*/}}
{{- define "rask-log-aggregator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "rask-log-aggregator.fullname" -}}
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
{{- define "rask-log-aggregator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "rask-log-aggregator.labels" -}}
helm.sh/chart: {{ include "rask-log-aggregator.chart" . }}
{{ include "rask-log-aggregator.selectorLabels" . }}
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
{{- define "rask-log-aggregator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rask-log-aggregator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "rask-log-aggregator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "rask-log-aggregator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "rask-log-aggregator.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Standard secret name template
*/}}
{{- define "rask-log-aggregator.secretName" -}}
{{- if .Values.envFromSecret.name }}
{{- .Values.envFromSecret.name }}
{{- else }}
{{- printf "%s-secrets" (include "rask-log-aggregator.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "rask-log-aggregator.envFromSecret" -}}
{{- if .Values.envFromSecret }}
{{- range .Values.envFromSecret.keys }}
- name: {{ . }}
  valueFrom:
    secretKeyRef:
      name: {{ include "rask-log-aggregator.secretName" $ }}
      key: {{ . }}
{{- end }}
{{- end }}
{{- end }}