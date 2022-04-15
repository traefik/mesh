---
title: "Traefik Mesh API"
description: "Traefik Mesh includes a built-in API that can be used for debugging purposes. read the documentation to learn more."
---

# API

Traefik Mesh includes a built-in API that can be used for debugging purposes.
This can be useful when Traefik Mesh is not working as intended.
The API is accessed via the controller pod, and for security reasons is not exposed via service.
The API can be accessed by making a `GET` request to `http://<control pod IP>:9000` combined with one of the following paths:

## `/api/configuration/current`

This endpoint provides raw json of the current configuration built by the controller.

!!! Note
    This may change on each request, as it is a live data structure.

## `/api/status/nodes`

This endpoint provides a json array containing some details about the readiness of the Traefik Mesh nodes visible by the controller.
This endpoint will still return a 200 if there are no visible nodes.

## `/api/status/node/{traefik-mesh-pod-name}/configuration`

This endpoint provides raw json of the current configuration on the Traefik Mesh node with the pod name given in `{traefik-mesh-pod-name}`.
This endpoint provides a 404 response if the pod cannot be found, or other non-200 status codes on other errors.
If errors are encountered, the error will be returned in the body, and logged on the controller.

## `/api/status/readiness`

This endpoint returns a 200 response if the controller has successfully started.
Otherwise, it will return a 500.
