
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
{{- define "maesh.controllerImage" -}}
    {{- printf "%s:%s" .Values.controller.image.name ( .Values.controller.image.tag | default .Chart.AppVersion ) -}}
{{- end -}}
