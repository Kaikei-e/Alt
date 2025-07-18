{{/*
Expand the name of the chart.
*/}}
{{- define "tag-generator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "tag-generator.fullname" -}}
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
{{- define "tag-generator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "tag-generator.labels" -}}
helm.sh/chart: {{ include "tag-generator.chart" . }}
{{ include "tag-generator.selectorLabels" . }}
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
{{- define "tag-generator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tag-generator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "tag-generator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "tag-generator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Standard secret name template
*/}}
{{- define "tag-generator.secretName" -}}
{{- if .Values.envFromSecret.name }}
{{- .Values.envFromSecret.name }}
{{- else }}
{{- printf "%s-secrets" (include "tag-generator.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets
*/}}
{{- define "tag-generator.envFromSecret" -}}
{{- if .Values.envFromSecret }}
{{- range .Values.envFromSecret.keys }}
- name: {{ . }}
  valueFrom:
    secretKeyRef:
      name: {{ include "tag-generator.secretName" $ }}
      key: {{ . }}
{{- end }}
{{- end }}
{{- end }}