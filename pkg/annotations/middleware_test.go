package annotations

import (
	"testing"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/stretchr/testify/assert"
)

func TestBuildMiddleware(t *testing.T) {
	tests := []struct {
		desc        string
		annotations map[string]string
		want        *dynamic.Middleware
		err         bool
	}{
		{
			desc:        "nil when no middleware have been created",
			annotations: map[string]string{},
		},
		{
			desc: "retry-attempts annotation is valid",
			annotations: map[string]string{
				"maesh.containo.us/retry-attempts": "5",
			},
			want: &dynamic.Middleware{
				Retry: &dynamic.Retry{
					Attempts: 5,
				},
			},
		},
		{
			desc: "retry-attempts annotation is invalid",
			annotations: map[string]string{
				"maesh.containo.us/retry-attempts": "hello",
			},
			err: true,
		},
		{
			desc: "circuit-breaker-expression",
			annotations: map[string]string{
				"maesh.containo.us/circuit-breaker-expression": "LatencyAtQuantileMS(50.0) > 100",
			},
			want: &dynamic.Middleware{
				CircuitBreaker: &dynamic.CircuitBreaker{
					Expression: "LatencyAtQuantileMS(50.0) > 100",
				},
			},
		},
		{
			desc: "ratelimit-average and ratelimit-burst are both valid",
			annotations: map[string]string{
				"maesh.containo.us/ratelimit-average": "200",
				"maesh.containo.us/ratelimit-burst":   "100",
			},
			want: &dynamic.Middleware{
				RateLimit: &dynamic.RateLimit{
					Average: 200,
					Burst:   100,
				},
			},
		},
		{
			desc: "ratelimit-average is valid but ratelimit-burst is invalid",
			annotations: map[string]string{
				"maesh.containo.us/ratelimit-average": "200",
				"maesh.containo.us/ratelimit-burst":   "hello",
			},
			err: true,
		},
		{
			desc: "ratelimit-burst is valid but ratelimit-average is invalid",
			annotations: map[string]string{
				"maesh.containo.us/ratelimit-burst":   "200",
				"maesh.containo.us/ratelimit-average": "hello",
			},
			err: true,
		},
		{
			desc: "ratelimit-average is set but ratelimit-burst is not",
			annotations: map[string]string{
				"maesh.containo.us/ratelimit-average": "200",
			},
		},
		{
			desc: "ratelimit-burst is set but ratelimit-average is not",
			annotations: map[string]string{
				"maesh.containo.us/ratelimit-burst": "200",
			},
		},
		{
			desc: "multiple middlewares",
			annotations: map[string]string{
				"maesh.containo.us/retry-attempts":             "5",
				"maesh.containo.us/ratelimit-average":          "200",
				"maesh.containo.us/ratelimit-burst":            "100",
				"maesh.containo.us/circuit-breaker-expression": "LatencyAtQuantileMS(50.0) > 100",
			},
			want: &dynamic.Middleware{
				Retry: &dynamic.Retry{
					Attempts: 5,
				},
				RateLimit: &dynamic.RateLimit{
					Average: 200,
					Burst:   100,
				},
				CircuitBreaker: &dynamic.CircuitBreaker{
					Expression: "LatencyAtQuantileMS(50.0) > 100",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := BuildMiddleware(test.annotations)
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.want, got)
			}
		})
	}
}
