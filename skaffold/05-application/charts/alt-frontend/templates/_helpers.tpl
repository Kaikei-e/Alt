{{/*
Expand the name of the chart.
*/}}
{{- define "alt-frontend.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "alt-frontend.fullname" -}}
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
{{- define "alt-frontend.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "alt-frontend.labels" -}}
helm.sh/chart: {{ include "alt-frontend.chart" . }}
{{ include "alt-frontend.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: alt
app.kubernetes.io/component: frontend
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "alt-frontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "alt-frontend.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "alt-frontend.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "alt-frontend.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Standard secret name template
*/}}
{{- define "alt-frontend.secretName" -}}
{{- if .Values.envFromSecret.name }}
{{- .Values.envFromSecret.name }}
{{- else }}
{{- printf "%s-secrets" (include "alt-frontend.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "alt-frontend.envFromSecret" -}}
{{- if .Values.envFromSecret }}
{{- range .Values.envFromSecret.keys }}
- name: {{ . }}
  valueFrom:
    secretKeyRef:
      name: {{ include "alt-frontend.secretName" $ }}
      key: {{ . }}
{{- end }}
{{- end }}
{{- end }}