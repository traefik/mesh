module github.com/containous/maesh

go 1.12

require (
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.4.2 // indirect
	github.com/Masterminds/sprig v2.20.0+incompatible // indirect
	github.com/cenkalti/backoff v2.1.1+incompatible
	github.com/containous/mux v0.0.0-20181024131434-c33f32e26898 // indirect
	github.com/containous/traefik v2.0.0-beta1.0.20190805162403-c2d440a914f4+incompatible
	github.com/deislabs/smi-sdk-go v0.0.0-20190621175932-114e91dce170
	github.com/go-acme/lego v2.6.0+incompatible // indirect
	github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/huandu/xstrings v1.2.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/vdemeester/shakers v0.1.0
	golang.org/x/crypto v0.0.0-20190621222207-cc06ce4a13d4 // indirect
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	golang.org/x/sys v0.0.0-20190624142023-c5567b49c5d0 // indirect
	golang.org/x/time v0.0.0-20190513212739-9d24e82272b4 // indirect
	google.golang.org/appengine v1.6.1 // indirect
	gopkg.in/square/go-jose.v2 v2.3.1 // indirect
	k8s.io/api v0.0.0-20190718183219-b59d8169aab5
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go v0.0.0-20190718183610-8e956561bbf5
	k8s.io/klog v0.3.3 // indirect
	k8s.io/utils v0.0.0-20190801114015-581e00157fb1 // indirect
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.1.0+incompatible
	github.com/go-resty/resty => gopkg.in/resty.v1 v1.9.1
	github.com/h2non/gock => gopkg.in/h2non/gock.v1 v1.0.14
	github.com/rancher/go-rancher-metadata => github.com/containous/go-rancher-metadata v0.0.0-20190402144056-c6a65f8b7a28
)
