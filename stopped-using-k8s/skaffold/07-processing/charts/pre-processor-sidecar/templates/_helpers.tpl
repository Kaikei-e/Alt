{{/*
Expand the name of the chart.
*/}}
{{- define "pre-processor-sidecar.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "pre-processor-sidecar.fullname" -}}
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
{{- define "pre-processor-sidecar.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "pre-processor-sidecar.labels" -}}
helm.sh/chart: {{ include "pre-processor-sidecar.chart" . }}
{{ include "pre-processor-sidecar.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: cronjob
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "pre-processor-sidecar.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pre-processor-sidecar.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "pre-processor-sidecar.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "pre-processor-sidecar.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Standard secret name template
*/}}
{{- define "pre-processor-sidecar.secretName" -}}
{{- if .Values.secret.name }}
{{- .Values.secret.name }}
{{- else }}
{{- printf "%s-secrets" (include "pre-processor-sidecar.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "pre-processor-sidecar.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "pre-processor-sidecar.envFromSecret" -}}
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