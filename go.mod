module github.com/containous/maesh

go 1.13

// Kubernetes version kubernetes-1.15.3
require (
	github.com/cenkalti/backoff/v3 v3.0.0
	github.com/containous/traefik/v2 v2.0.0-rc1
	github.com/deislabs/smi-sdk-go v0.0.0-20190819154013-e53a9b2d8c1a
	github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/gogo/protobuf v1.2.2-0.20190730201129-28a6bbf47e48 // indirect
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/json-iterator/go v1.1.7 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/vdemeester/shakers v0.1.0
	golang.org/x/crypto v0.0.0-20190820162420-60c769a6c586 // indirect
	golang.org/x/net v0.0.0-20190813141303-74dc4d7220e7 // indirect
	golang.org/x/sys v0.0.0-20190825160603-fb81701db80f // indirect
	google.golang.org/appengine v1.6.1 // indirect
	k8s.io/api v0.0.0-20190819141258-3544db3b9e44
	k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go v0.0.0-20190819141724-e14f31a72a77
	k8s.io/klog v0.3.3 // indirect
	k8s.io/utils v0.0.0-20190801114015-581e00157fb1 // indirect
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.1.0+incompatible
	github.com/docker/docker => github.com/docker/engine v0.0.0-20190725163905-fa8dd90ceb7b
	github.com/go-resty/resty => gopkg.in/resty.v1 v1.9.1
	github.com/h2non/gock => gopkg.in/h2non/gock.v1 v1.0.14
)

// Containous forks
replace (
	github.com/abbot/go-http-auth => github.com/containous/go-http-auth v0.4.1-0.20180112153951-65b0cdae8d7f
	github.com/go-check/check => github.com/containous/check v0.0.0-20170915194414-ca0bf163426a
	github.com/gorilla/mux => github.com/containous/mux v0.0.0-20181024131434-c33f32e26898
	github.com/mailgun/minheap => github.com/containous/minheap v0.0.0-20190809180810-6e71eb837595
	github.com/mailgun/multibuf => github.com/containous/multibuf v0.0.0-20190809014333-8b6c9a7e6bba
	github.com/rancher/go-rancher-metadata => github.com/containous/go-rancher-metadata v0.0.0-20190402144056-c6a65f8b7a28
)
