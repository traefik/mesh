[global]
checkNewVersion = false
sendAnonymousUsage = false

[log]
level = "DEBUG"

[entryPoints]
  [entryPoints.web]
    address = ":8000"

[providers]
   [providers.file]

[http.routers]
{{- range $key, $service := .Services }}
  [http.routers.{{ $service.ServiceName}}_{{ $service.ServiceNamespace }}]
    entrypoints = ["web"]
    rule = "Host(`{{ $service.ServiceName }}.{{ $service.ServiceNamespace }}.traefik.mesh`)"
    service = "{{ $service.ServiceName}}_{{ $service.ServiceNamespace }}"
{{- end }}

[http.services]
{{- range $key, $service := .Services }}
  [http.services.{{ $service.ServiceName}}_{{ $service.ServiceNamespace }}.loadbalancer]
    {{- range $subkey, $server := $service.Servers }}
    [[http.services.{{ $service.ServiceName}}_{{ $service.ServiceNamespace }}.loadbalancer.servers]]
      url = "http://{{ $server.Address }}:{{ $server.Port }}"
      weight = 1
    {{- end -}}
{{- end -}}
