module github.com/containous/maesh

go 1.14

require (
	github.com/abronan/valkeyrie v0.0.0-20200127174252-ef4277a138cd
	github.com/cenkalti/backoff/v4 v4.0.2
	github.com/containous/traefik/v2 v2.2.3
	github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/hashicorp/go-version v1.2.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/servicemeshinterface/smi-sdk-go v0.3.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.5.1
	github.com/vdemeester/shakers v0.1.0
	github.com/vulcand/oxy v1.1.0
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v0.17.4
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.1.0+incompatible
	github.com/docker/docker => github.com/docker/engine v1.4.2-0.20200204220554-5f6d6f3f2203
)

// Containous forks
replace (
	github.com/abbot/go-http-auth => github.com/containous/go-http-auth v0.4.1-0.20200324110947-a37a7636d23e
	github.com/go-check/check => github.com/containous/check v0.0.0-20170915194414-ca0bf163426a
	github.com/gorilla/mux => github.com/containous/mux v0.0.0-20200408164629-f779179d490a
	github.com/mailgun/minheap => github.com/containous/minheap v0.0.0-20190809180810-6e71eb837595
	github.com/mailgun/multibuf => github.com/containous/multibuf v0.0.0-20190809014333-8b6c9a7e6bba
)
