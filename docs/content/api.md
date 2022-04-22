---
title: "Traefik Mesh API"
description: "Traefik Mesh includes a built-in API that can be used for debugging purposes. Read the documentation to learn more."
---

# API

Traefik Mesh includes a built-in API that can be used for debugging purposes.
This can be useful when Traefik Mesh is not working as intended.
The API is accessed via the controller pod, and for security reasons is not exposed via service.
The API can be accessed by making a `GET` request to `http://<control pod IP>:9000` combined with one of the following paths:

## `/api/configuration`

This endpoint provides raw json of the current configuration built by the controller.

!!! Note
    This may change on each request, as it is a live data structure.

## `/api/topology`

This endpoint provides raw json of the current topology built by the controller.

!!! Note
    This may change on each request, as it is a live data structure.


## `/api/ready`

This endpoint returns a 200 response if the controller has successfully started.
Otherwise, it will return a 500.
