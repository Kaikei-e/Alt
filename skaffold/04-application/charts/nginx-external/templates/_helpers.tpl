{{/*
Expand the name of the chart.
*/}}
{{- define "nginx-external.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "nginx-external.fullname" -}}
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
{{- define "nginx-external.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nginx-external.labels" -}}
helm.sh/chart: {{ include "nginx-external.chart" . }}
{{ include "nginx-external.selectorLabels" . }}
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
{{- define "nginx-external.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nginx-external.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nginx-external.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nginx-external.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the configmap to use
*/}}
{{- define "nginx-external.configMapName" -}}
{{- if .Values.configMap.create }}
{{- default (include "nginx-external.fullname" .) .Values.configMap.name }}
{{- else }}
{{- .Values.configMap.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use
*/}}
{{- define "nginx-external.secretName" -}}
{{- if .Values.secret.create }}
{{- default (include "nginx-external.fullname" .) .Values.secret.name }}
{{- else }}
{{- .Values.secret.name }}
{{- end }}
{{- end }}

{{/*
Generate nginx upstream configuration
*/}}
{{- define "nginx-external.upstreams" -}}
{{- range .Values.upstreams }}
upstream {{ .name }} {
    {{- if .loadBalancing }}
    {{ .loadBalancing }};
    {{- end }}
    {{- if .keepalive }}
    keepalive {{ .keepalive }};
    {{- end }}
    {{- range .servers }}
    server {{ . }};
    {{- end }}
    {{- if .healthCheck }}
    {{- if .healthCheck.enabled }}
    # Health check configuration
    # health_check interval={{ .healthCheck.interval | default "30s" }}
    #             fails={{ .healthCheck.fails | default "3" }}
    #             passes={{ .healthCheck.passes | default "2" }};
    {{- end }}
    {{- end }}
}
{{- end }}
{{- end }}

{{/*
Generate nginx rate limiting zones
*/}}
{{- define "nginx-external.rateLimitZones" -}}
{{- with .Values.nginx }}
# Rate limiting zones for external traffic
limit_req_zone $binary_remote_addr zone=general:10m rate={{ .rateLimitRps | default 20 }}r/s;
limit_req_zone $binary_remote_addr zone=api:10m rate={{ div (.rateLimitRpm | default 600) 60 }}r/s;
limit_conn_zone $binary_remote_addr zone=addr:10m;
{{- end }}
{{- end }}

{{/*
Generate nginx server configuration
*/}}
{{- define "nginx-external.servers" -}}
{{- range .Values.servers }}
server {
    {{- range .listen }}
    listen {{ . }};
    {{- end }}
    {{- if .serverName }}
    server_name {{ .serverName }};
    {{- end }}

    {{- if .ssl }}
    # SSL Configuration
    ssl_certificate {{ .ssl.certificate }};
    ssl_certificate_key {{ .ssl.certificateKey }};
    ssl_protocols {{ .ssl.protocols }};
    ssl_ciphers {{ .ssl.ciphers }};
    ssl_prefer_server_ciphers off;
    {{- if .ssl.sessionCache }}
    ssl_session_cache {{ .ssl.sessionCache }};
    {{- end }}
    {{- if .ssl.sessionTimeout }}
    ssl_session_timeout {{ .ssl.sessionTimeout }};
    {{- end }}
    {{- if .ssl.sessionTickets }}
    ssl_session_tickets {{ .ssl.sessionTickets }};
    {{- end }}
    {{- if .ssl.ocspStapling }}
    ssl_stapling {{ .ssl.ocspStapling }};
    {{- end }}
    {{- if .ssl.ocspStaplingVerify }}
    ssl_stapling_verify {{ .ssl.ocspStaplingVerify }};
    {{- end }}
    {{- end }}

    {{- range .locations }}
    location {{ .path }} {
        {{- if .proxyPass }}
        proxy_pass {{ .proxyPass }};
        {{- range .proxySetHeader }}
        proxy_set_header {{ . }};
        {{- end }}
        {{- if .proxyTimeout }}
        proxy_connect_timeout {{ .proxyTimeout }};
        proxy_send_timeout {{ .proxyTimeout }};
        proxy_read_timeout {{ .proxyTimeout }};
        {{- end }}
        {{- end }}
        {{- if .extraConfig }}
        {{ .extraConfig | nindent 8 }}
        {{- end }}
    }
    {{- end }}

    {{- if .extraConfig }}
    {{ .extraConfig | nindent 4 }}
    {{- end }}
}
{{- end }}
{{- end }}

{{/*
Generate nginx geoip configuration if enabled
*/}}
{{- define "nginx-external.geoip" -}}
{{- if .Values.nginx.geoBlocking }}
{{- if .Values.nginx.geoBlocking.enabled }}
# GeoIP blocking configuration
geo $allowed_country {
    default 0;
    {{- range .Values.nginx.geoBlocking.allowedCountries }}
    {{ . }} 1;
    {{- end }}
}

geo $blocked_country {
    default 0;
    {{- range .Values.nginx.geoBlocking.blockedCountries }}
    {{ . }} 1;
    {{- end }}
}
{{- end }}
{{- end }}
{{- end }}

{{/*
Generate nginx cache configuration
*/}}
{{- define "nginx-external.cache" -}}
{{- if .Values.nginx.cache }}
{{- if .Values.nginx.cache.enabled }}
# Cache configuration
proxy_cache_path {{ .Values.nginx.cache.path }}
                 levels={{ .Values.nginx.cache.levels }}
                 keys_zone={{ .Values.nginx.cache.keysZone }}
                 max_size={{ .Values.nginx.cache.maxSize }}
                 inactive={{ .Values.nginx.cache.inactive }};
{{- end }}
{{- end }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "nginx-external.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Pod annotations
*/}}
{{- define "nginx-external.podAnnotations" -}}
{{- with .Values.podAnnotations }}
{{ toYaml . }}
{{- end }}
{{- if .Values.monitoring.enabled }}
{{- with .Values.monitoring.annotations }}
{{ toYaml . }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Security context
*/}}
{{- define "nginx-external.securityContext" -}}
{{- toYaml .Values.securityContext }}
{{- end }}

{{/*
Pod security context
*/}}
{{- define "nginx-external.podSecurityContext" -}}
{{- toYaml .Values.podSecurityContext }}
{{- end }}

{{/*
Image pull secrets
*/}}
{{- define "nginx-external.imagePullSecrets" -}}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
{{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Environment variables from secret
*/}}
{{- define "nginx-external.envFromSecret" -}}
{{- if .Values.envFromSecret.name }}
envFrom:
- secretRef:
    name: {{ .Values.envFromSecret.name }}
{{- end }}
{{- end }}

{{/*
Environment variables
*/}}
{{- define "nginx-external.env" -}}
{{- with .Values.env }}
env:
{{- range $key, $value := . }}
- name: {{ $key }}
  value: {{ $value | quote }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Volumes
*/}}
{{- define "nginx-external.volumes" -}}
{{- with .Values.volumes }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Volume mounts
*/}}
{{- define "nginx-external.volumeMounts" -}}
{{- with .Values.volumeMounts }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Node selector
*/}}
{{- define "nginx-external.nodeSelector" -}}
{{- with .Values.nodeSelector }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Affinity
*/}}
{{- define "nginx-external.affinity" -}}
{{- with .Values.affinity }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Tolerations
*/}}
{{- define "nginx-external.tolerations" -}}
{{- with .Values.tolerations }}
{{ toYaml . }}
{{- end }}
{{- end }}