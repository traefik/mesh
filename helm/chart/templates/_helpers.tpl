
{{/* vim: set filetype=mustache: */}}

{{/*
Define the Chart version Label
*/}}
{{- define "i3o.chartLabel" -}}
    {{- printf "%s-%s" .Chart.Name .Chart.Version -}}
{{- end -}}

{{/*
Define the templated image with tag
*/}}
{{- define "i3o.image" -}}
    {{- printf "%s:%s" .Values.image.name ( .Values.image.tag | default .Chart.AppVersion ) -}}
{{- end -}}
