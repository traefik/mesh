
{{/* vim: set filetype=mustache: */}}

{{/*
Define the Chart version Label
*/}}
{{- define "maesh.chartLabel" -}}
    {{- printf "%s-%s" .Chart.Name .Chart.Version -}}
{{- end -}}

{{/*
Define the templated image with tag
*/}}
{{- define "maesh.image" -}}
    {{- printf "%s:%s" .Values.image.name ( .Values.image.tag | default .Chart.AppVersion ) -}}
{{- end -}}
