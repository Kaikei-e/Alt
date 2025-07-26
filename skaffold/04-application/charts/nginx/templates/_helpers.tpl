{{/*
Expand the name of the chart.
*/}}
{{- define "nginx.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "nginx.fullname" -}}
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
{{- define "nginx.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nginx.labels" -}}
helm.sh/chart: {{ include "nginx.chart" . }}
{{ include "nginx.selectorLabels" . }}
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
{{- define "nginx.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nginx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nginx.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nginx.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Generate nginx upstream configuration for services
*/}}
{{- define "nginx.upstreams" -}}
{{- range .Values.upstreams }}
upstream {{ .name }} {
{{- range .servers }}
    server {{ . }};
{{- end }}
{{- if .loadBalancing }}
    {{ .loadBalancing }};
{{- end }}
{{- if .keepalive }}
    keepalive {{ .keepalive }};
{{- end }}
}
{{- end }}
{{- end }}

{{/*
Generate nginx server blocks
*/}}
{{- define "nginx.servers" -}}
{{- range .Values.servers }}
server {
{{- if .listen }}
{{- range .listen }}
    listen {{ . }};
{{- end }}
{{- end }}
{{- if .serverName }}
    server_name {{ .serverName }};
{{- end }}
{{- if .ssl }}
{{- if .ssl.certificate }}
    ssl_certificate {{ .ssl.certificate }};
{{- end }}
{{- if .ssl.certificateKey }}
    ssl_certificate_key {{ .ssl.certificateKey }};
{{- end }}
{{- if .ssl.protocols }}
    ssl_protocols {{ .ssl.protocols }};
{{- end }}
{{- if .ssl.ciphers }}
    ssl_ciphers {{ .ssl.ciphers }};
{{- end }}
{{- end }}
{{- if .locations }}
{{- range .locations }}
    location {{ .path }} {
{{- if .proxyPass }}
        proxy_pass {{ .proxyPass }};
{{- end }}
{{- if .proxySetHeader }}
{{- range .proxySetHeader }}
        proxy_set_header {{ . }};
{{- end }}
{{- end }}
{{- if .proxyTimeout }}
        proxy_connect_timeout {{ .proxyTimeout }};
        proxy_send_timeout {{ .proxyTimeout }};
        proxy_read_timeout {{ .proxyTimeout }};
{{- end }}
{{- if .extraConfig }}
{{ .extraConfig | indent 8 }}
{{- end }}
    }
{{- end }}
{{- end }}
{{- if .extraConfig }}
{{ .extraConfig | indent 4 }}
{{- end }}
}
{{- end }}
{{- end }}

{{/*
Create configmap name
*/}}
{{- define "nginx.configMapName" -}}
{{- if .Values.configMap.name }}
{{- .Values.configMap.name }}
{{- else }}
{{- include "nginx.fullname" . }}-config
{{- end }}
{{- end }}

{{/*
Create secret name
*/}}
{{- define "nginx.secretName" -}}
{{- if .Values.secret.name }}
{{- .Values.secret.name }}
{{- else }}
{{- include "nginx.fullname" . }}-secret
{{- end }}
{{- end }}