module github.com/containous/i3o

go 1.12

require (
	github.com/containous/traefik v1.7.12
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	k8s.io/api v0.0.0-20190602205700-9b8cae951d65
	k8s.io/apimachinery v0.0.0-20190606174813-5a6182816fbf
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/klog v0.3.2
	k8s.io/sample-controller v0.0.0-20190531134801-325dc0a18ed9
	k8s.io/utils v0.0.0-20190529001817-6999998975a7 // indirect
)

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190425172711-65184652c889
