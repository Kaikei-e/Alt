{{/*
Expand the name of the chart.
*/}}
{{- define "pre-processor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "pre-processor.fullname" -}}
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
{{- define "pre-processor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "pre-processor.labels" -}}
helm.sh/chart: {{ include "pre-processor.chart" . }}
{{ include "pre-processor.selectorLabels" . }}
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
{{- define "pre-processor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pre-processor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "pre-processor.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "pre-processor.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Standard secret name template
*/}}
{{- define "pre-processor.secretName" -}}
{{- if .Values.envFromSecret.name }}
{{- .Values.envFromSecret.name }}
{{- else }}
{{- printf "%s-secrets" (include "pre-processor.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "pre-processor.envFromSecret" -}}
{{- if .Values.envFromSecret }}
{{- range .Values.envFromSecret.keys }}
- name: {{ . }}
  valueFrom:
    secretKeyRef:
      name: {{ include "pre-processor.secretName" $ }}
      key: {{ . }}
{{- end }}
{{- end }}
{{- end }}