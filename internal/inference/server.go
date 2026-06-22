package inference

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/service"
)

type Control interface {
	Status(context.Context) (service.Status, error)
	ListInstances(context.Context) ([]service.InstanceSummary, error)
}

type Validator interface {
	Validate(string) (bool, error)
}

func NewHandler(cfg config.InferenceConfig, control Control, validator Validator) http.Handler {
	basePath := "/" + strings.Trim(strings.TrimSpace(cfg.BasePath), "/")
	if basePath == "/" {
		basePath = ""
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if basePath != "" && r.URL.Path != basePath && !strings.HasPrefix(r.URL.Path, basePath+"/") {
			http.NotFound(w, r)
			return
		}
		if !authorized(r, validator) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		target, err := activeTarget(r.Context(), control)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		original := proxy.Director
		proxy.Director = func(req *http.Request) {
			original(req)
			req.URL.Path = joinPath(target.Path, strings.TrimPrefix(r.URL.Path, basePath))
			req.URL.RawQuery = r.URL.RawQuery
			req.Host = target.Host
		}
		proxy.ServeHTTP(w, r)
	})
}

func Serve(ctx context.Context, cfg config.InferenceConfig, control Control, validator Validator) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.DevInsecureHTTP && !isLoopbackAddress(cfg.Address) {
		return fmt.Errorf("dev_insecure_http requires loopback inference address")
	}
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           NewHandler(cfg, control, validator),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		if cfg.DevInsecureHTTP {
			errc <- server.ListenAndServe()
			return
		}
		errc <- server.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	}()
	select {
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func authorized(r *http.Request, validator Validator) bool {
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return false
	}
	valid, err := validator.Validate(token)
	return err == nil && valid
}

func activeTarget(ctx context.Context, control Control) (*url.URL, error) {
	status, err := control.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.ActiveProfile == "" {
		return nil, fmt.Errorf("no active profile")
	}
	instances, err := control.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		if instance.ProfileID == status.ActiveProfile && instance.State == "running" {
			return url.Parse(instance.Endpoint)
		}
	}
	return nil, fmt.Errorf("active profile is not running")
}

func joinPath(base, suffix string) string {
	if base == "" {
		base = "/"
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address == "localhost" || strings.HasPrefix(address, "127.") || address == "::1"
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
