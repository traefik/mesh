module github.com/containous/i3o

go 1.12

require (
	github.com/cenkalti/backoff v2.1.1+incompatible
	github.com/containous/mux v0.0.0-20181024131434-c33f32e26898 // indirect
	github.com/containous/traefik v2.0.0-alpha5+incompatible
	github.com/deislabs/smi-sdk-go v0.0.0-20190621175932-114e91dce170
	github.com/go-acme/lego v2.6.0+incompatible // indirect
	github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.3.0
	github.com/vdemeester/shakers v0.1.0
	golang.org/x/crypto v0.0.0-20190621222207-cc06ce4a13d4 // indirect
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	golang.org/x/sys v0.0.0-20190621203818-d432491b9138 // indirect
	google.golang.org/appengine v1.6.1 // indirect
	gopkg.in/square/go-jose.v2 v2.3.1 // indirect
	k8s.io/api v0.0.0-20190620073856-dcce3486da33
	k8s.io/apimachinery v0.0.0-20190620073744-d16981aedf33
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/klog v0.3.3 // indirect
	k8s.io/sample-controller v0.0.0-20190620075113-d4d703847b23
	k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190602205700-9b8cae951d65
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190606174813-5a6182816fbf
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190531132438-d58e65e5f4b1
	k8s.io/sample-controller => k8s.io/sample-controller v0.0.0-20190531134801-325dc0a18ed9
)
