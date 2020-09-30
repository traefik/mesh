package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResourceFilter_New(t *testing.T) {
	var (
		firstOptionCallCounter  int
		secondOptionCallCounter int
	)

	_ = NewResourceFilter(func(filter *ResourceFilter) {
		firstOptionCallCounter++
	}, func(filter *ResourceFilter) {
		secondOptionCallCounter++
	})

	assert.Equal(t, 1, firstOptionCallCounter)
	assert.Equal(t, 1, secondOptionCallCounter)
}

func TestResourceFilter_IsIgnoredWithNoRestriction(t *testing.T) {
	filter := NewResourceFilter()

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
	})

	assert.False(t, got)
}

func TestResourceFilter_IsIgnoredCalledWithUnexpectedType(t *testing.T) {
	filter := NewResourceFilter()

	got := filter.IsIgnored(1)

	assert.True(t, got)
}

func TestResourceFilter_IsIgnoredWithIgnoredNamespaces(t *testing.T) {
	filter := NewResourceFilter()
	filter.ignoredNamespaces = []string{"ignored-ns"}

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ignored-ns"},
	})

	assert.True(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
	})

	assert.False(t, got)
}

func TestResourceFilter_IsIgnoredWithWatchedNamespaces(t *testing.T) {
	filter := NewResourceFilter()
	filter.watchedNamespaces = []string{"watched-ns"}

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
	})

	assert.True(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "watched-ns"},
	})

	assert.False(t, got)
}

func TestResourceFilter_IsIgnoredWithWatchedNamespacesAndIgnoredNamespaces(t *testing.T) {
	filter := NewResourceFilter()
	filter.watchedNamespaces = []string{"ns-1", "ns-2"}
	filter.ignoredNamespaces = []string{"ns-2", "ns-3"}

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-1"},
	})

	assert.False(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-2"},
	})

	assert.True(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-3"},
	})

	assert.True(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-4"},
	})

	assert.True(t, got)
}

func TestResourceFilter_IsIgnoredWithIgnoredLabels(t *testing.T) {
	filter := NewResourceFilter()
	filter.ignoredLabels = map[string]string{
		"foo": "bar",
	}

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	})

	assert.True(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"bar": "baz",
			},
		},
	})

	assert.False(t, got)
}

func TestResourceFilter_IsIgnoredWithIgnoredServices(t *testing.T) {
	filter := NewResourceFilter()
	filter.ignoredServices = []namespaceName{{Namespace: "ns-1", Name: "svc-1"}}

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-1",
		},
	})

	assert.True(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-2",
			Name:      "svc-1",
		},
	})

	assert.False(t, got)

	got = filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-2",
		},
	})

	assert.False(t, got)

	got = filter.IsIgnored(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-1",
		},
	})

	assert.False(t, got)
}

func TestResourceFilter_IsIgnoredIgnoresExternalNameServices(t *testing.T) {
	filter := NewResourceFilter()

	got := filter.IsIgnored(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-1",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeExternalName,
		},
	})

	assert.True(t, got)
}

func TestResourceFilter_WatchNamespaces(t *testing.T) {
	filter := NewResourceFilter()

	WatchNamespaces("ns-1", "ns-2")(filter)

	assert.Equal(t, []string{"ns-1", "ns-2"}, filter.watchedNamespaces)
	assert.Len(t, filter.ignoredNamespaces, 0)
	assert.Len(t, filter.ignoredServices, 0)
	assert.Len(t, filter.ignoredLabels, 0)
}

func TestResourceFilter_IgnoreNamespaces(t *testing.T) {
	filter := NewResourceFilter()

	IgnoreNamespaces("ns-1", "ns-2")(filter)

	assert.Equal(t, []string{"ns-1", "ns-2"}, filter.ignoredNamespaces)
	assert.Len(t, filter.watchedNamespaces, 0)
	assert.Len(t, filter.ignoredServices, 0)
	assert.Len(t, filter.ignoredLabels, 0)
}

func TestResourceFilter_IgnoreApps(t *testing.T) {
	filter := NewResourceFilter()

	IgnoreLabel("foo", "bar")(filter)

	assert.Equal(t, map[string]string{"foo": "bar"}, filter.ignoredLabels)
	assert.Len(t, filter.ignoredNamespaces, 0)
	assert.Len(t, filter.watchedNamespaces, 0)
	assert.Len(t, filter.ignoredServices, 0)
}

func TestResourceFilter_IgnoreService(t *testing.T) {
	filter := NewResourceFilter()

	IgnoreService("ns-1", "svc-1")(filter)

	assert.Equal(t, []namespaceName{{Namespace: "ns-1", Name: "svc-1"}}, filter.ignoredServices)
	assert.Len(t, filter.ignoredNamespaces, 0)
	assert.Len(t, filter.watchedNamespaces, 0)
	assert.Len(t, filter.ignoredLabels, 0)
}
