package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/elegba-dev/elegba/internal/config"
)

const designExampleToken = "Bearer client-token"

func TestDesignWorkedExampleLoadsAndAggregates(t *testing.T) {
	var userCalls, ledgerCalls, accountCalls atomic.Int64
	userServer := newDesignExampleUpstream(t, "/users/42", `{"id":42,"user_status":"ACTIVE","date_create":"2024-03-15T10:22:00Z"}`, &userCalls, "user-service", designExampleToken)
	defer userServer.Close()
	ledgerServer := newDesignExampleUpstream(t, "/ledger/accounts/42", `{"account_number":"A-100","ledger_balance":1580.42}`, &ledgerCalls, "ledger-service", designExampleToken)
	defer ledgerServer.Close()
	accountServer := newDesignExampleUpstream(t, "/accounts/42/flags", `{"has_pnd":true,"has_lien":true}`, &accountCalls, "account-service", designExampleToken)
	defer accountServer.Close()

	cfg := loadDesignExampleConfig(t, userServer.URL, ledgerServer.URL, accountServer.URL)
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	request := httptest.NewRequest(http.MethodGet, "/users/42/summary", nil)
	request.Header.Set("Authorization", designExampleToken)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	want := map[string]any{
		"user_id":           float64(42),
		"user_status":       "ACTIVE",
		"user_onboarded_on": "2024-03-15T10:22:00Z",
		"account_balance":   1580.42,
		"account_has_pnd":   true,
		"account_has_lien":  true,
	}
	for key, wantValue := range want {
		if result[key] != wantValue {
			t.Fatalf("%s = %#v, want %#v (body %s)", key, result[key], wantValue, response.Body.String())
		}
	}
	if got := userCalls.Load(); got != 1 {
		t.Fatalf("user upstream calls = %d, want 1", got)
	}
	if got := ledgerCalls.Load(); got != 1 {
		t.Fatalf("ledger upstream calls = %d, want 1", got)
	}
	if got := accountCalls.Load(); got != 1 {
		t.Fatalf("account upstream calls = %d, want 1", got)
	}
}

func TestDesignWorkedExampleCacheIsTokenIsolated(t *testing.T) {
	var userCalls atomic.Int64
	userServer := newDesignExampleUpstream(t, "/users/42", `{"id":42,"user_status":"ACTIVE","date_create":"2024-03-15T10:22:00Z"}`, &userCalls, "user-service", "")
	defer userServer.Close()
	ledgerServer := newDesignExampleUpstream(t, "/ledger/accounts/42", `{"account_number":"A-100","ledger_balance":1580.42}`, new(atomic.Int64), "ledger-service", "")
	defer ledgerServer.Close()
	accountServer := newDesignExampleUpstream(t, "/accounts/42/flags", `{"has_pnd":true,"has_lien":true}`, new(atomic.Int64), "account-service", "")
	defer accountServer.Close()

	cfg := loadDesignExampleConfig(t, userServer.URL, ledgerServer.URL, accountServer.URL)
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	for _, token := range []string{"Bearer first-token", "Bearer second-token"} {
		request := httptest.NewRequest(http.MethodGet, "/users/42/summary", nil)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status for %q = %d, body = %s", token, response.Code, response.Body.String())
		}
	}
	if got := userCalls.Load(); got != 2 {
		t.Fatalf("user upstream calls = %d, want 2 for token-isolated cache keys", got)
	}
}

func TestDesignWorkedExampleReturnsClientAuthMissing(t *testing.T) {
	userServer := newDesignExampleUpstream(t, "/users/42", `{"id":42}`, new(atomic.Int64), "user-service", "")
	defer userServer.Close()
	ledgerServer := newDesignExampleUpstream(t, "/ledger/accounts/42", `{}`, new(atomic.Int64), "ledger-service", "")
	defer ledgerServer.Close()
	accountServer := newDesignExampleUpstream(t, "/accounts/42/flags", `{}`, new(atomic.Int64), "account-service", "")
	defer accountServer.Close()

	cfg := loadDesignExampleConfig(t, userServer.URL, ledgerServer.URL, accountServer.URL)
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42/summary", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var responseError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &responseError); err != nil {
		t.Fatal(err)
	}
	if responseError.Error.Code != "CLIENT_AUTH_MISSING" {
		t.Fatalf("error code = %q, want CLIENT_AUTH_MISSING", responseError.Error.Code)
	}
}

func TestDesignWorkedExampleAccountFallback(t *testing.T) {
	var accountCalls atomic.Int64
	userServer := newDesignExampleUpstream(t, "/users/42", `{"id":42,"user_status":"ACTIVE","date_create":"2024-03-15T10:22:00Z"}`, new(atomic.Int64), "user-service", designExampleToken)
	defer userServer.Close()
	ledgerServer := newDesignExampleUpstream(t, "/ledger/accounts/42", `{"account_number":"A-100","ledger_balance":1580.42}`, new(atomic.Int64), "ledger-service", designExampleToken)
	defer ledgerServer.Close()
	accountServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		accountCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer accountServer.Close()

	cfg := loadDesignExampleConfig(t, userServer.URL, ledgerServer.URL, accountServer.URL)
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	request := httptest.NewRequest(http.MethodGet, "/users/42/summary", nil)
	request.Header.Set("Authorization", designExampleToken)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["account_has_pnd"] != false || result["account_has_lien"] != false {
		t.Fatalf("fallback result = %#v", result)
	}
	if got := accountCalls.Load(); got != 4 {
		t.Fatalf("account upstream calls = %d, want 4 (initial attempt plus 3 retries)", got)
	}
}

func TestDesignWorkedExampleTransformsNestedResponse(t *testing.T) {
	var profileCalls, activityCalls atomic.Int64
	profileUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		profileCalls.Add(1)
		if req.URL.Path != "/profiles/42" {
			t.Errorf("profile upstream path = %q, want %q", req.URL.Path, "/profiles/42")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":{"id":42,"name":"Ada","roles":["admin","reader"]}}`))
	}))
	defer profileUpstream.Close()

	activityUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		activityCalls.Add(1)
		if req.URL.Path != "/activity/42" {
			t.Errorf("activity upstream path = %q, want %q", req.URL.Path, "/activity/42")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"activity":{"id":42,"status":"active","score":9.5}}`))
	}))
	defer activityUpstream.Close()

	t.Setenv("ELEGBA_TEST_NESTED_PROFILE_URL", profileUpstream.URL)
	t.Setenv("ELEGBA_TEST_NESTED_ACTIVITY_URL", activityUpstream.URL)
	cfg, err := config.LoadConfig(filepath.Join("testdata", "nested-response.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/profiles/42", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id":          float64(42),
		"name":        "Ada",
		"roles":       []any{"admin", "reader"},
		"activity_id": float64(42),
		"status":      "active",
		"score":       9.5,
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("flattened response = %#v, want %#v", result, want)
	}
	if got := profileCalls.Load(); got != 1 {
		t.Fatalf("profile upstream calls = %d, want 1", got)
	}
	if got := activityCalls.Load(); got != 1 {
		t.Fatalf("activity upstream calls = %d, want 1", got)
	}
}

func newDesignExampleUpstream(t *testing.T, path, body string, calls *atomic.Int64, name, expectedToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.Method != http.MethodGet || req.URL.Path != path {
			t.Errorf("%s received method=%s path=%s", name, req.Method, req.URL.Path)
		}
		if expectedToken != "" && req.Header.Get("Authorization") != expectedToken {
			t.Errorf("%s Authorization = %q, want %q", name, req.Header.Get("Authorization"), expectedToken)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func loadDesignExampleConfig(t *testing.T, userURL, ledgerURL, accountURL string) *config.Config {
	t.Helper()
	t.Setenv("ELEGBA_TEST_USER_SERVICE_URL", userURL)
	t.Setenv("ELEGBA_TEST_LEDGER_SERVICE_URL", ledgerURL)
	t.Setenv("ELEGBA_TEST_ACCOUNT_SERVICE_URL", accountURL)
	cfg, err := config.LoadConfig(filepath.Join("testdata", "design-16.3.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
