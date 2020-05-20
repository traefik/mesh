package annotations

import (
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
				"maesh.containo.us/traffic-type": "hello",
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
				"maesh.containo.us/traffic-type": "http",
			},
			want: ServiceTypeHTTP,
		},
		{
			desc: "tcp",
			annotations: map[string]string{
				"maesh.containo.us/traffic-type": "tcp",
			},
			want: ServiceTypeTCP,
		},
		{
			desc: "udp",
			annotations: map[string]string{
				"maesh.containo.us/traffic-type": "udp",
			},
			want: ServiceTypeUDP,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tt, err := GetTrafficType(ServiceTypeHTTP, test.annotations)
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.want, tt)
			}
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
			desc: "unknown service type",
			annotations: map[string]string{
				"maesh.containo.us/scheme": "hello",
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
				"maesh.containo.us/scheme": "http",
			},
			want: SchemeHTTP,
		},
		{
			desc: "https",
			annotations: map[string]string{
				"maesh.containo.us/scheme": "https",
			},
			want: SchemeHTTPS,
		},
		{
			desc: "h2c",
			annotations: map[string]string{
				"maesh.containo.us/scheme": "h2c",
			},
			want: SchemeH2C,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tt, err := GetScheme(test.annotations)
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.want, tt)
			}
		})
	}
}

func TestGetRetryAttempts_Valid(t *testing.T) {
	attempts, err := GetRetryAttempts(map[string]string{
		"maesh.containo.us/retry-attempts": "2",
	})

	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestGetRetryAttempts_NotSet(t *testing.T) {
	_, err := GetRetryAttempts(map[string]string{})

	assert.Equal(t, ErrNotFound, err)
}

func TestGetRetryAttempts_Invalid(t *testing.T) {
	_, err := GetRetryAttempts(map[string]string{
		"maesh.containo.us/retry-attempts": "hello",
	})

	assert.Error(t, err)
}

func TestGetCircuitBreakerExpression_Valid(t *testing.T) {
	expression, err := GetCircuitBreakerExpression(map[string]string{
		"maesh.containo.us/circuit-breaker-expression": "LatencyAtQuantileMS(50.0) > 100",
	})

	require.NoError(t, err)
	assert.Equal(t, "LatencyAtQuantileMS(50.0) > 100", expression)
}

func TestGetCircuitBreakerExpression_NotSet(t *testing.T) {
	_, err := GetCircuitBreakerExpression(map[string]string{})

	assert.Equal(t, ErrNotFound, err)
}

func TestGetRateLimitBurst_Valid(t *testing.T) {
	attempts, err := GetRateLimitBurst(map[string]string{
		"maesh.containo.us/ratelimit-burst": "200",
	})

	require.NoError(t, err)
	assert.Equal(t, 200, attempts)
}

func TestGetRateLimitBurst_NotSet(t *testing.T) {
	_, err := GetRateLimitBurst(map[string]string{})

	assert.Equal(t, ErrNotFound, err)
}

func TestGetRateLimitBurst_Invalid(t *testing.T) {
	_, err := GetRateLimitBurst(map[string]string{
		"maesh.containo.us/ratelimit-burst": "hello",
	})

	assert.Error(t, err)
}

func TestGetRateLimitAverage_Valid(t *testing.T) {
	attempts, err := GetRateLimitAverage(map[string]string{
		"maesh.containo.us/ratelimit-average": "100",
	})

	require.NoError(t, err)
	assert.Equal(t, 100, attempts)
}

func TestGetRateLimitAverage_NotSet(t *testing.T) {
	_, err := GetRateLimitAverage(map[string]string{})

	assert.Equal(t, ErrNotFound, err)
}

func TestGetRateLimitAverage_Invalid(t *testing.T) {
	_, err := GetRateLimitAverage(map[string]string{
		"maesh.containo.us/ratelimit-average": "hello",
	})

	assert.Error(t, err)
}
