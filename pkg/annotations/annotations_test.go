package annotations

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTrafficType(t *testing.T) {
	tests := []struct {
		desc        string
		annotations map[string]string
		want        string
		err         bool
	}{
		{
			desc: "unknown service type",
			annotations: map[string]string{
				"mesh.traefik.io/traffic-type": "hello",
			},
			err: true,
		},
		{
			desc:        "returns the default traffic-type if not set",
			annotations: map[string]string{},
			want:        ServiceTypeHTTP,
		},
		{
			desc: "http",
			annotations: map[string]string{
				"mesh.traefik.io/traffic-type": "http",
			},
			want: ServiceTypeHTTP,
		},
		{
			desc: "tcp",
			annotations: map[string]string{
				"mesh.traefik.io/traffic-type": "tcp",
			},
			want: ServiceTypeTCP,
		},
		{
			desc: "udp",
			annotations: map[string]string{
				"mesh.traefik.io/traffic-type": "udp",
			},
			want: ServiceTypeUDP,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tt, err := GetTrafficType(ServiceTypeHTTP, test.annotations)
			if test.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, tt)
		})
	}
}

func TestGetScheme(t *testing.T) {
	tests := []struct {
		desc        string
		annotations map[string]string
		want        string
		err         bool
	}{
		{
			desc: "unknown scheme",
			annotations: map[string]string{
				"mesh.traefik.io/scheme": "hello",
			},
			err: true,
		},
		{
			desc:        "returns the default scheme if not set",
			annotations: map[string]string{},
			want:        SchemeHTTP,
		},
		{
			desc: "http",
			annotations: map[string]string{
				"mesh.traefik.io/scheme": "http",
			},
			want: SchemeHTTP,
		},
		{
			desc: "https",
			annotations: map[string]string{
				"mesh.traefik.io/scheme": "https",
			},
			want: SchemeHTTPS,
		},
		{
			desc: "h2c",
			annotations: map[string]string{
				"mesh.traefik.io/scheme": "h2c",
			},
			want: SchemeH2C,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			scheme, err := GetScheme(test.annotations)
			if test.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, scheme)
		})
	}
}

func TestGetRetryAttempts(t *testing.T) {
	tests := []struct {
		desc         string
		annotations  map[string]string
		want         int
		err          bool
		wantNotFound bool
	}{
		{
			desc: "invalid",
			annotations: map[string]string{
				"mesh.traefik.io/retry-attempts": "hello",
			},
			err: true,
		},
		{
			desc: "valid",
			annotations: map[string]string{
				"mesh.traefik.io/retry-attempts": "2",
			},
			want: 2,
		},
		{
			desc:         "not set",
			annotations:  map[string]string{},
			err:          true,
			wantNotFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			attempts, err := GetRetryAttempts(test.annotations)
			if test.err {
				require.Error(t, err)
				assert.Equal(t, test.wantNotFound, errors.Is(err, ErrNotFound))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, attempts)
		})
	}
}

func TestGetCircuitBreakerExpression(t *testing.T) {
	tests := []struct {
		desc         string
		annotations  map[string]string
		want         string
		err          bool
		wantNotFound bool
	}{
		{
			desc: "valid",
			annotations: map[string]string{
				"mesh.traefik.io/circuit-breaker-expression": "LatencyAtQuantileMS(50.0) > 100",
			},
			want: "LatencyAtQuantileMS(50.0) > 100",
		},
		{
			desc:         "not set",
			annotations:  map[string]string{},
			err:          true,
			wantNotFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			value, err := GetCircuitBreakerExpression(test.annotations)
			if test.err {
				require.Error(t, err)
				assert.Equal(t, test.wantNotFound, errors.Is(err, ErrNotFound))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, value)
		})
	}
}

func TestGetRateLimitBurst(t *testing.T) {
	tests := []struct {
		desc         string
		annotations  map[string]string
		want         int
		err          bool
		wantNotFound bool
	}{
		{
			desc: "invalid",
			annotations: map[string]string{
				"mesh.traefik.io/ratelimit-burst": "hello",
			},
			err: true,
		},
		{
			desc: "valid",
			annotations: map[string]string{
				"mesh.traefik.io/ratelimit-burst": "200",
			},
			want: 200,
		},
		{
			desc:         "not set",
			annotations:  map[string]string{},
			err:          true,
			wantNotFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			value, err := GetRateLimitBurst(test.annotations)
			if test.err {
				require.Error(t, err)
				assert.Equal(t, test.wantNotFound, errors.Is(err, ErrNotFound))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, value)
		})
	}
}

func TestGetRateLimitAverage(t *testing.T) {
	tests := []struct {
		desc         string
		annotations  map[string]string
		want         int
		err          bool
		wantNotFound bool
	}{
		{
			desc: "invalid",
			annotations: map[string]string{
				"mesh.traefik.io/ratelimit-average": "hello",
			},
			err: true,
		},
		{
			desc: "valid",
			annotations: map[string]string{
				"mesh.traefik.io/ratelimit-average": "100",
			},
			want: 100,
		},
		{
			desc:         "not set",
			annotations:  map[string]string{},
			err:          true,
			wantNotFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			value, err := GetRateLimitAverage(test.annotations)
			if test.err {
				require.Error(t, err)
				assert.Equal(t, test.wantNotFound, errors.Is(err, ErrNotFound))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, value)
		})
	}
}

func Test_getAnnotation(t *testing.T) {
	tests := []struct {
		desc        string
		annotations map[string]string
		name        string
		want        string
		exists      bool
	}{
		{
			desc: "should return the traefik mesh annotation value",
			annotations: map[string]string{
				"mesh.traefik.io/foo": "bar",
			},
			name:   "foo",
			want:   "bar",
			exists: true,
		},
		{
			desc: "should return the deprecated maesh annotation value",
			annotations: map[string]string{
				"maesh.containo.us/foo": "bar",
			},
			name:   "foo",
			want:   "bar",
			exists: true,
		},
		{
			desc: "should return the traefik mesh annotation value",
			annotations: map[string]string{
				"mesh.traefik.io/foo":   "bar",
				"maesh.containo.us/foo": "fuzzy",
			},
			name:   "foo",
			want:   "bar",
			exists: true,
		},
		{
			desc: "should return not found",
			annotations: map[string]string{
				"mesh.traefik.io/foo":   "bar",
				"maesh.containo.us/foo": "fuzzy",
			},
			name: "bar",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			value, exists := getAnnotation(test.annotations, test.name)

			assert.Equal(t, test.want, value)
			assert.Equal(t, test.exists, exists)
		})
	}
}
