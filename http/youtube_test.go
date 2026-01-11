package http

import (
	"net/http"
	"testing"
)

func TestYouTubeRateLimitDetector(t *testing.T) {
	detector := NewYouTubeRateLimitDetector()

	tests := []struct {
		name       string
		statusCode int
		header     http.Header
		expected   bool
	}{
		{
			name:       "429 TooManyRequests",
			statusCode: http.StatusTooManyRequests,
			header:     make(http.Header),
			expected:   true,
		},
		{
			name:       "503 ServiceUnavailable",
			statusCode: http.StatusServiceUnavailable,
			header:     make(http.Header),
			expected:   true,
		},
		{
			name:       "200 OK",
			statusCode: http.StatusOK,
			header:     make(http.Header),
			expected:   false,
		},
		{
			name:       "403 with Retry-After",
			statusCode: http.StatusForbidden,
			header: http.Header{
				"Retry-After": []string{"60"},
			},
			expected: true,
		},

		{
			name:       "403 without rate limit headers",
			statusCode: http.StatusForbidden,
			header:     make(http.Header),
			expected:   false,
		},
		{
			name:       "404 NotFound",
			statusCode: http.StatusNotFound,
			header:     make(http.Header),
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.IsRateLimited(tc.statusCode, tc.header)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGetRetryAfterDuration(t *testing.T) {
	detector := NewYouTubeRateLimitDetector()

	tests := []struct {
		name     string
		header   http.Header
		expected int64
	}{
		{
			name: "Retry-After header",
			header: http.Header{
				"Retry-After": []string{"120"},
			},
			expected: 120,
		},
		{
			name: "X-RateLimit-Reset header",
			header: http.Header{
				"X-RateLimit-Reset": []string{"60"},
			},
			expected: 60,
		},
		{
			name:     "No rate limit header",
			header:   make(http.Header),
			expected: 60, // Default
		},
		{
			name: "Multiple headers, first wins",
			header: http.Header{
				"Retry-After":        []string{"120"},
				"X-RateLimit-Reset":  []string{"60"},
			},
			expected: 120,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.GetRetryAfterDuration(tc.header)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestIsClientError(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{http.StatusBadRequest, true},      // 400
		{http.StatusUnauthorized, true},    // 401
		{http.StatusForbidden, true},       // 403
		{http.StatusNotFound, true},        // 404
		{http.StatusTooManyRequests, true}, // 429
		{http.StatusOK, false},             // 200
		{http.StatusCreated, false},        // 201
		{http.StatusInternalServerError, false}, // 500
		{http.StatusServiceUnavailable, false},  // 503
	}

	for _, tc := range tests {
		result := IsClientError(tc.statusCode)
		if result != tc.expected {
			t.Errorf("IsClientError(%d): expected %v, got %v", tc.statusCode, tc.expected, result)
		}
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{http.StatusInternalServerError, true}, // 500
		{http.StatusBadGateway, true},          // 502
		{http.StatusServiceUnavailable, true},  // 503
		{http.StatusGatewayTimeout, true},      // 504
		{http.StatusOK, false},                 // 200
		{http.StatusNotFound, false},           // 404
		{http.StatusTooManyRequests, false},    // 429
	}

	for _, tc := range tests {
		result := IsServerError(tc.statusCode)
		if result != tc.expected {
			t.Errorf("IsServerError(%d): expected %v, got %v", tc.statusCode, tc.expected, result)
		}
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		// Server errors (5xx) should retry
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},

		// Specific client errors should retry
		{http.StatusRequestTimeout, true},      // 408
		{http.StatusTooManyRequests, true},     // 429
		{http.StatusServiceUnavailable, true},  // 503
		{http.StatusGatewayTimeout, true},      // 504

		// Success codes should not retry
		{http.StatusOK, false},
		{http.StatusCreated, false},
		{http.StatusAccepted, false},

		// Other client errors should not retry
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
	}

	for _, tc := range tests {
		result := ShouldRetry(tc.statusCode)
		if result != tc.expected {
			t.Errorf("ShouldRetry(%d): expected %v, got %v", tc.statusCode, tc.expected, result)
		}
	}
}
