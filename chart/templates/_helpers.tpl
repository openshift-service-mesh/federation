{{/*
Expand the name of the chart.
*/}}
{{- define "chart.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "chart.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "chart.serviceName" -}}
{{- printf "federation-discovery-service-%s" (default .Release.Name .Values.federation.meshPeers.local.name) }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "chart.labels" -}}
helm.sh/chart: {{ include "chart.chart" . }}
{{ include "chart.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "chart.selectorLabels" -}}
app.kubernetes.io/name: {{ include "chart.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Checks if any of the remotes have ingress type of openshift-router
*/}}
{{- define "remotes.hasOpenshiftRouterPeer" -}}
{{- $remotes := .Values.federation.meshPeers.remotes | default list -}}
{{- $found := false -}}
{{- range $remotes }}
  {{- if eq .ingressType "openshift-router" }}
    {{- $found = true -}}
  {{- end }}
{{- end }}
{{- $found -}}
{{- end -}}
