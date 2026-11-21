package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"
)

type webAPI struct {
	app          *AppService
	expectedHost string
	shutdown     func()
}

type apiResponse struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func newWebHandler(app *AppService, expectedHost string, frontend fs.FS, shutdown func()) http.Handler {
	api := &webAPI{app: app, expectedHost: expectedHost, shutdown: shutdown}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", api.get(func() any {
		return map[string]string{"status": "ok", "version": desktopVersion}
	}))
	mux.HandleFunc("/api/snapshot", api.get(func() any { return app.GetSnapshot() }))
	mux.HandleFunc("/api/profiles", api.get(func() any { return app.GetProfiles() }))
	mux.HandleFunc("/api/settings", api.method(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeAPIResponse(w, http.StatusOK, app.GetSettings())
	}))
	mux.HandleFunc("/api/settings/save", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Settings AppSettings `json:"settings"`
		}
		if err := decodeAPIJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := app.SaveSettings(input.Settings); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, input.Settings)
	}))
	mux.HandleFunc("/api/profiles/save", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Profile Profile `json:"profile"`
		}
		if err := decodeAPIJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		profiles, err := app.SaveProfile(input.Profile)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, profiles)
	}))
	mux.HandleFunc("/api/profiles/active", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID string `json:"id"`
		}
		if err := decodeAPIJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := app.SetActiveProfile(input.ID); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"saved": true})
	}))
	mux.HandleFunc("/api/profiles/delete", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID string `json:"id"`
		}
		if err := decodeAPIJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		profiles, err := app.DeleteProfile(input.ID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, profiles)
	}))
	mux.HandleFunc("/api/engine/install", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		engine, err := app.InstallOrUpdateEngine()
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, engine)
	}))
	mux.HandleFunc("/api/tunnel/start", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if err := app.StartTunnel(); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"started": true})
	}))
	mux.HandleFunc("/api/tunnel/stop", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if err := app.StopTunnel(); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"stopped": true})
	}))
	mux.HandleFunc("/api/tunnel/restart", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if err := app.RestartTunnel(); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"restarted": true})
	}))
	mux.HandleFunc("/api/background/start", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if err := app.StartBackground(); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"started": true})
	}))
	mux.HandleFunc("/api/background/stop", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if err := app.StopBackground(); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"stopped": true})
	}))
	mux.HandleFunc("/api/background/remove", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if err := app.RemoveBackground(); err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, map[string]bool{"removed": true})
	}))
	mux.HandleFunc("/api/diagnostics", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		results := app.RunDiagnostics()
		writeAPIResponse(w, http.StatusOK, results)
	}))
	mux.HandleFunc("/api/logs", api.get(func() any {
		logs := app.GetLogs()
		if logs == nil { return []LogEntry{} }
		return logs
	}))
	mux.HandleFunc("/api/logs/clear", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		app.ClearLogs()
		writeAPIResponse(w, http.StatusOK, map[string]bool{"cleared": true})
	}))
	mux.HandleFunc("/api/shutdown", api.method(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		writeAPIResponse(w, http.StatusAccepted, map[string]bool{"shuttingDown": true})
		if api.shutdown != nil {
			go func() {
				time.Sleep(150 * time.Millisecond)
				api.shutdown()
			}()
		}
	}))

	mux.Handle("/", http.FileServer(http.FS(frontend)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRemote(r.RemoteAddr) {
			http.Error(w, "loopback access only", http.StatusForbidden)
			return
		}
		if r.Host != expectedHost {
			http.Error(w, "invalid host", http.StatusMisdirectedRequest)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+expectedHost {
			http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *webAPI) get(fn func() any) http.HandlerFunc {
	return a.method(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeAPIResponse(w, http.StatusOK, fn())
	})
}

func (a *webAPI) method(want string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != want {
			w.Header().Set("Allow", want)
			writeAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed", r.Method))
			return
		}
		fn(w, r)
	}
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func decodeAPIJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeAPIResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{Data: data})
}

func writeAPIError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{Error: err.Error()})
}
