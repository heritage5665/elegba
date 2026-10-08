package adminserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminHandlerRegistersMetricsHealthAndPprof(t *testing.T) {
	handler := NewHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }),
	)
	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/metrics", want: http.StatusNoContent},
		{path: "/healthz", want: http.StatusOK},
		{path: "/readyz", want: http.StatusServiceUnavailable},
		{path: "/debug/pprof/", want: http.StatusOK},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.want {
			t.Errorf("%s: expected status %d, got %d", test.path, test.want, response.Code)
		}
	}
}
