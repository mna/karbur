package kick_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mna/kick"
)

var (
	localhostCert string
	localhostKey  string

	nonRandomPort int
)

//nolint:gochecknoinits
func init() {
	localhostCert = os.Getenv("KICK_TEST_LOCALHOST_CERT")
	localhostKey = os.Getenv("KICK_TEST_LOCALHOST_KEY")
	if localhostCert == "" || localhostKey == "" {
		panic("localhost TLS certificate or key not set")
	}

	nonRandomPort = 9000
}

func nextPort() int {
	nonRandomPort++
	return nonRandomPort
}

func TestServer_HTTP2(t *testing.T) {
	const (
		timeout      = time.Second
		requestAfter = 500 * time.Millisecond
	)

	var port = nextPort()

	s := kick.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.NotFoundHandler(),
		TLS: &kick.TLSConfig{
			CertFile: localhostCert,
			KeyFile:  localhostKey,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	var lasErr error
	wg.Add(1)
	go func() {
		defer wg.Done()

		lasErr = s.ListenAndServe(ctx)
	}()

	time.Sleep(requestAfter)
	res, err := http.Get(fmt.Sprintf("https://localhost:%d/", port))
	if err != nil {
		t.Fatalf("want no client error, got %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 404 {
		t.Fatalf("want status code 404, got %d", res.StatusCode)
	}
	if res.ProtoMajor < 2 {
		t.Fatalf("want http/2, got %s", res.Proto)
	}

	cancel()
	wg.Wait()
	if want := context.Canceled; lasErr != want {
		t.Fatalf("want ListenAndServe error to be %v; got %v", want, lasErr)
	}
}

func TestServer_HTTP2Disabled(t *testing.T) {
	const (
		timeout      = time.Second
		requestAfter = 500 * time.Millisecond
	)

	var port = nextPort()

	s := kick.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.NotFoundHandler(),
		TLS: &kick.TLSConfig{
			CertFile:     localhostCert,
			KeyFile:      localhostKey,
			DisableHTTP2: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	var lasErr error
	wg.Add(1)
	go func() {
		defer wg.Done()

		lasErr = s.ListenAndServe(ctx)
	}()

	time.Sleep(requestAfter)
	res, err := http.Get(fmt.Sprintf("https://localhost:%d/", port))
	if err != nil {
		t.Fatalf("want no client error, got %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 404 {
		t.Fatalf("want status code 404, got %d", res.StatusCode)
	}
	if res.ProtoMajor >= 2 {
		t.Fatalf("want http/1.1, got %s", res.Proto)
	}

	cancel()
	wg.Wait()
	if want := context.Canceled; lasErr != want {
		t.Fatalf("want ListenAndServe error to be %v; got %v", want, lasErr)
	}
}

func TestServer_TLS(t *testing.T) {
	const (
		timeout      = time.Second
		requestAfter = 500 * time.Millisecond
	)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("ok"))
	})

	modes := []kick.TLSMode{kick.TLSDefault, kick.TLSIntermediate, kick.TLSModern}
	for _, m := range modes {
		t.Run(m.String(), func(t *testing.T) {
			var port = nextPort()

			s := kick.Server{
				Addr:    fmt.Sprintf(":%d", port),
				Handler: h,
				TLS: &kick.TLSConfig{
					Mode:     m,
					CertFile: localhostCert,
					KeyFile:  localhostKey,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			var wg sync.WaitGroup
			var lasErr error
			wg.Add(1)
			go func() {
				defer wg.Done()

				lasErr = s.ListenAndServe(ctx)
			}()

			time.Sleep(requestAfter)
			res, err := http.Get(fmt.Sprintf("https://localhost:%d/", port))
			if err != nil {
				t.Fatalf("want no client error, got %s", err)
			}
			defer res.Body.Close()

			if res.StatusCode != 404 {
				t.Fatalf("want status code 404, got %d", res.StatusCode)
			}
			b, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %s", err)
			}
			if string(b) != "ok" {
				t.Fatalf(`want response "ok", got %s`, string(b))
			}
			cancel()
			wg.Wait()
			if want := context.Canceled; lasErr != want {
				t.Fatalf("want ListenAndServe error to be %v; got %v", want, lasErr)
			}
		})
	}
}

func TestServer_GracefulShutdownSignal(t *testing.T) {
	const (
		shutdownAfter   = time.Second
		signalAfter     = 100 * time.Millisecond
		shutdownTimeout = 300 * time.Millisecond
		replyAfter      = 200 * time.Millisecond
	)
	port := nextPort()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(replyAfter)
		_, _ = w.Write([]byte("ok"))
	})

	waitForListen := make(chan struct{})
	s := kick.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h,
		GracefulShutdown: &kick.GracefulShutdownConfig{
			Timeout: shutdownTimeout,
			Signals: []os.Signal{syscall.SIGUSR1},
		},
		ServerStateHook: func(_ *http.Server, state kick.ServerState) {
			if state == kick.StateListening {
				close(waitForListen)
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownAfter)
	defer cancel()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to get current process: %s", err)
	}

	var wg sync.WaitGroup
	var dur time.Duration
	var lasErr error
	wg.Add(2)
	go func() {
		start := time.Now()
		lasErr = s.ListenAndServe(ctx)
		dur = time.Since(start)
		wg.Done()
	}()
	go func() {
		time.Sleep(signalAfter)
		if err := proc.Signal(syscall.SIGUSR1); err != nil {
			panic(fmt.Sprintf("failed to send signal: %s", err))
		}
		wg.Done()
	}()

	<-waitForListen
	res, err := http.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err != nil {
		t.Fatalf("want no client error, got %s", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to ready response body: %s", err)
	}
	if string(body) != "ok" {
		t.Fatalf(`want body to be "ok", got %q`, string(body))
	}

	wg.Wait()
	if want := context.Canceled; lasErr != want {
		t.Fatalf("want server error %v; got %v", want, lasErr)
	}
	minDur := signalAfter
	maxDur := minDur + shutdownTimeout
	if dur < minDur || dur > maxDur {
		t.Fatalf("want server duration %s, got %s", minDur, dur)
	}
}

func TestServer_GracefulShutdownCtx(t *testing.T) {
	const (
		shutdownAfter   = 200 * time.Millisecond
		shutdownTimeout = 200 * time.Millisecond
		replyAfter      = 300 * time.Millisecond
	)
	port := nextPort()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(replyAfter)
		_, _ = w.Write([]byte("ok"))
	})
	s := kick.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h,
		GracefulShutdown: &kick.GracefulShutdownConfig{
			Timeout: shutdownTimeout,
			Signals: []os.Signal{os.Interrupt},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownAfter)
	defer cancel()

	var wg sync.WaitGroup
	var dur time.Duration
	var lasErr error
	wg.Add(1)
	go func() {
		start := time.Now()
		lasErr = s.ListenAndServe(ctx)
		dur = time.Since(start)
		wg.Done()
	}()

	res, err := http.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err != nil {
		t.Fatalf("want no client error, got %s", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to ready response body: %s", err)
	}
	if string(body) != "ok" {
		t.Fatalf(`want body to be "ok", got %q`, string(body))
	}

	wg.Wait()
	if lasErr != context.DeadlineExceeded {
		t.Fatalf("want server error %s, got %s", context.DeadlineExceeded, lasErr)
	}
	minDur := shutdownAfter
	maxDur := minDur + shutdownTimeout
	if dur < minDur || dur > maxDur {
		t.Fatalf("want server duration %s, got %s", minDur, dur)
	}
}

func TestServer_ListenAndServeFail(t *testing.T) {
	// start a listener on a random port, so that the server cannot
	// use it later on.
	l, err := net.Listen("tcp", "")
	if err != nil {
		t.Fatalf("dummy server failed to listen: %s", err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to parse port from dummy address: %s", err)
	}
	defer l.Close()

	// start a server on the same port, will fail on ListenAndServe
	s := kick.Server{
		Addr: fmt.Sprintf(":%s", port),
	}

	const timeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	err = s.ListenAndServe(ctx)
	dur := time.Since(start)
	if err == nil {
		t.Fatalf("want error, got none")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("unexpected error message: %s", err)
	}
	if dur >= timeout {
		t.Fatalf("should've failed immediately, took %s", dur)
	}
}

func TestServer_ServerState(t *testing.T) {
	const timeout = 100 * time.Millisecond

	var mu sync.Mutex
	var states string
	state := func(srv *http.Server, st kick.ServerState) {
		mu.Lock()
		states += st.String()
		mu.Unlock()
	}

	s := kick.Server{
		Addr:            ":0",
		ServerStateHook: state,
		GracefulShutdown: &kick.GracefulShutdownConfig{
			Timeout: time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// should start and stop properly until cancelled
	err := s.ListenAndServe(ctx)
	if want := context.DeadlineExceeded; err != want {
		t.Fatalf("want ListeAndServe error %v; got %v", want, err)
	}
	const wantStates = "StateNewStateListeningStateShutdownStateClosed"
	if states != wantStates {
		t.Fatalf("want states %s, got %s", wantStates, states)
	}
}

func TestServer_HTTPServer(t *testing.T) {
	s := kick.Server{}

	srv, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	if srv == nil {
		t.Fatalf("want configured server, got nil")
	}

	// build has been called by HTTPServer
	err = s.Build()
	if err == nil {
		t.Fatalf("want error, got none")
	}
	if !strings.Contains(err.Error(), "already built") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestServer_Build(t *testing.T) {
	s := kick.Server{}

	err := s.Build()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}

	// build a second time returns an error
	err = s.Build()
	if err == nil {
		t.Fatalf("want error, got none")
	}
	if !strings.Contains(err.Error(), "already built") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestServer_ZeroValue(t *testing.T) {
	var s kick.Server

	// make sure it doesn't run forever if it doesn't behave as expected
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := s.ListenAndServe(ctx)
	if err == nil {
		t.Fatalf("want error, got none")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestServer_Default(t *testing.T) {
	s := kick.Server{
		Addr:    ":0",
		Handler: http.NotFoundHandler(),
	}

	const timeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// should start and stop properly until cancelled
	start := time.Now()
	err := s.ListenAndServe(ctx)
	dur := time.Since(start)
	if want := context.DeadlineExceeded; err != want {
		t.Fatalf("want ListenAndServe error %v; got %v", want, err)
	}
	minDur := timeout - time.Millisecond
	if dur < minDur || dur >= 2*minDur {
		t.Fatalf("should've run for %s, got %s", timeout, dur)
	}

	ctx, cancel = context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// should fail on restart
	err = s.ListenAndServe(ctx)
	if err != http.ErrServerClosed {
		t.Fatalf("want %s, got %s", http.ErrServerClosed, err)
	}
}

func TestTLS_Intermediate(t *testing.T) {
	s := &kick.Server{
		TLS: &kick.TLSConfig{
			AutoCert: true,
			Mode:     kick.TLSIntermediate,
		},
	}

	hs, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	_ = hs // TODO: assert some tls configs
}

func TestTLS_Modern(t *testing.T) {
	s := &kick.Server{
		TLS: &kick.TLSConfig{
			AutoCert: true,
			Mode:     kick.TLSModern,
		},
	}

	hs, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	if hs.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("want MinVersion to be %v, got %v", tls.VersionTLS12, hs.TLSConfig.MinVersion)
	}
}

func TestTLS_InvalidMode(t *testing.T) {
	s := &kick.Server{
		TLS: &kick.TLSConfig{
			AutoCert: true,
			Mode:     kick.TLSModern + 1000,
		},
	}

	_, err := s.HTTPServer()
	if err == nil {
		t.Fatalf("want error, got none")
	}
	if !strings.Contains(err.Error(), "tls: unsupported TLS mode") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestTLS_AutoCert(t *testing.T) {
	s := &kick.Server{
		TLS: &kick.TLSConfig{
			AutoCert: true,
		},
	}

	hs, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	if hs.TLSConfig.GetCertificate == nil {
		t.Fatalf("want GetCertificate to be set")
	}
}

func TestTLS_ValidCert(t *testing.T) {
	s := &kick.Server{
		TLS: &kick.TLSConfig{
			CertFile: os.Getenv("KICK_TEST_LOCALHOST_CERT"),
			KeyFile:  os.Getenv("KICK_TEST_LOCALHOST_KEY"),
		},
	}

	hs, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	if len(hs.TLSConfig.Certificates) != 1 {
		t.Fatalf("want 1 certificate, got %d", len(hs.TLSConfig.Certificates))
	}
}

func TestTLS_InvalidCert(t *testing.T) {
	s := &kick.Server{
		TLS: &kick.TLSConfig{
			CertFile: os.Getenv("KICK_TEST_LOCALHOST_CERT"),
		},
	}

	_, err := s.HTTPServer()
	if err == nil {
		t.Fatalf("want error, got none")
	}
	if !strings.Contains(err.Error(), "tls: certificate and key files") {
		t.Fatalf("unexpected error message: %s", err)
	}
}

func TestTLS_NoConfig(t *testing.T) {
	s := new(kick.Server)
	hs, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	if hs.TLSConfig != nil {
		t.Fatalf("want no config, got %v", hs.TLSConfig)
	}
}

func TestHTTPServer(t *testing.T) {
	s := &kick.Server{
		Addr: ":1234",
	}
	hs, err := s.HTTPServer()
	if err != nil {
		t.Fatalf("want no error, got %s", err)
	}
	if hs.Addr != s.Addr {
		t.Fatalf("want address %s, got %s", s.Addr, hs.Addr)
	}
}
