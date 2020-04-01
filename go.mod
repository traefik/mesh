module github.com/containous/maesh

go 1.13

// Kubernetes version kubernetes-1.15.3
require (
	github.com/abronan/valkeyrie v0.0.0-20200127174252-ef4277a138cd
	github.com/cenkalti/backoff/v4 v4.0.0
	github.com/containous/traefik/v2 v2.2.0
	github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/pmezard/go-difflib v1.0.0
	github.com/servicemeshinterface/smi-sdk-go v0.3.1-0.20200326101714-d0668c95e1dc
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.5.1
	github.com/vdemeester/shakers v0.1.0
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v0.18.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.1.0+incompatible
	github.com/docker/docker => github.com/docker/engine v0.0.0-20190725163905-fa8dd90ceb7b
)

// Containous forks
replace (
	github.com/abbot/go-http-auth => github.com/containous/go-http-auth v0.4.1-0.20180112153951-65b0cdae8d7f
	github.com/go-check/check => github.com/containous/check v0.0.0-20170915194414-ca0bf163426a
	github.com/mailgun/minheap => github.com/containous/minheap v0.0.0-20190809180810-6e71eb837595
	github.com/mailgun/multibuf => github.com/containous/multibuf v0.0.0-20190809014333-8b6c9a7e6bba
	github.com/rancher/go-rancher-metadata => github.com/containous/go-rancher-metadata v0.0.0-20190402144056-c6a65f8b7a28
)
