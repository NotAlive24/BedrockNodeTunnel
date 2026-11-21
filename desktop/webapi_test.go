package main

import (
	"encoding/json"
	"io"
	"strings"
	"time"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoopbackListenerBindsOnlyIPv4Loopback(t *testing.T) {
	listener, err := listenLoopback()
	if err != nil {
		t.Fatalf("listenLoopback: %v", err)
	}
	defer listener.Close()

	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("invalid listener address %q: %v", listener.Addr(), err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("listener bound to %q; want 127.0.0.1", host)
	}
}

func TestIsLoopbackRemote(t *testing.T) {
	tests := []struct {
		address string
		want     bool
	}{
		{"127.0.0.1:12345", true},
		{"127.0.0.2:12345", true},
		{"[::1]:12345", true},
		{"192.0.2.10:12345", false},
		{"0.0.0.0:12345", false},
		{"not-an-address", false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := isLoopbackRemote(test.address); got != test.want {
				t.Fatalf("isLoopbackRemote(%q) = %v, want %v", test.address, got, test.want)
			}
		})
	}
}

func testAppService(t *testing.T) *AppService {
	t.Helper()
	root := t.TempDir()
	return &AppService{
		configDir:  root,
		configPath: root + "/desktop.json",
		config: appConfig{
			SchemaVersion: 1,
			ActiveProfile: "default",
			Profiles:      []Profile{defaultProfile()},
		},
		connection: "Stopped",
	}
}

func testWebHandler(t *testing.T, shutdown func()) http.Handler {
	t.Helper()
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>Nethernet</title>")},
	}
	return newWebHandler(testAppService(t), "127.0.0.1:34567", assets, shutdown)
}

func localRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, "http://127.0.0.1:34567"+path, nil)
	request.Host = "127.0.0.1:34567"
	request.RemoteAddr = "127.0.0.1:45678"
	return request
}

func TestWebHandlerServesFrontendOnLoopback(t *testing.T) {
	handler := testWebHandler(t, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, localRequest(http.MethodGet, "/"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Nethernet") {
		t.Fatalf("frontend content missing: %q", recorder.Body.String())
	}
	if recorder.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS access must not be enabled")
	}
}

func TestWebHandlerHealthAPI(t *testing.T) {
	handler := testWebHandler(t, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, localRequest(http.MethodGet, "/api/health"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/health status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Data.Status != "ok" || response.Data.Version != desktopVersion {
		t.Fatalf("unexpected health response: %+v", response.Data)
	}
}

func TestWebHandlerRejectsNonLoopbackAndHostSpoofing(t *testing.T) {
	handler := testWebHandler(t, nil)

	t.Run("non-loopback peer", func(t *testing.T) {
		request := localRequest(http.MethodGet, "/api/health")
		request.RemoteAddr = "192.0.2.10:45678"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
		}
	})

	t.Run("unexpected host", func(t *testing.T) {
		request := localRequest(http.MethodGet, "/api/health")
		request.Host = "attacker.example"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMisdirectedRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMisdirectedRequest)
		}
	})

	t.Run("cross-origin", func(t *testing.T) {
		request := localRequest(http.MethodGet, "/api/health")
		request.Header.Set("Origin", "https://attacker.example")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
		}
	})
}

func TestWebHandlerRequiresCorrectMethod(t *testing.T) {
	handler := testWebHandler(t, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, localRequest(http.MethodPost, "/api/health"))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow header = %q, want GET", recorder.Header().Get("Allow"))
	}
}

func TestWebHandlerSavesProfileThroughJSONAPI(t *testing.T) {
	service := testAppService(t)
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<title>Nethernet</title>")},
	}
	handler := newWebHandler(service, "127.0.0.1:34567", assets, nil)
	body := `{"profile":{"id":"","name":"Test Server","region":"AUTO","localTCP":"127.0.0.1:19132","localUDPIP":"","localUDPPort":47334}}`
	request := localRequest(http.MethodPost, "/api/profiles/save")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:34567")
	request.Body = io.NopCloser(strings.NewReader(body))
	request.ContentLength = int64(len(body))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save profile status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if len(service.GetProfiles()) != 2 {
		t.Fatalf("profile count = %d, want 2", len(service.GetProfiles()))
	}
	if service.GetProfiles()[1].Name != "Test Server" {
		t.Fatalf("saved profile name = %q, want Test Server", service.GetProfiles()[1].Name)
	}
}

func TestWebHandlerRejectsMalformedJSONAndUnknownFields(t *testing.T) {
	handler := testWebHandler(t, nil)
	tests := []string{
		`{"profile":`,
		`{"profile":{"name":"Home","localTCP":"127.0.0.1:19132","localUDPPort":47334,"unexpected":true}}`,
	}
	for _, body := range tests {
		request := localRequest(http.MethodPost, "/api/profiles/save")
		request.Header.Set("Content-Type", "application/json")
		request.Body = io.NopCloser(strings.NewReader(body))
		request.ContentLength = int64(len(body))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %q status = %d, want %d", body, recorder.Code, http.StatusBadRequest)
		}
	}
}

func TestWebHandlerShutdownCallback(t *testing.T) {
	called := make(chan struct{}, 1)
	handler := testWebHandler(t, func() { called <- struct{}{} })
	request := localRequest(http.MethodPost, "/api/shutdown")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("shutdown status = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not called")
	}
}
