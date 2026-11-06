package tlsutil

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestDualListenerServesTLSAndRedirectsPlainHTTP(t *testing.T) {
	cfg, _, _, err := GenerateSelfSigned("localhost")
	if err != nil {
		t.Fatal(err)
	}
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := NewDualListener(inner, cfg)
	// As in `hitstor serve`: the server's TLSConfig must offer h2 too,
	// or net/http does not enable HTTP/2 while the listener advertises it.
	srv := &http.Server{TLSConfig: ln.TLSConfig(), Handler: RedirectPlainHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "secure "+r.URL.Path)
	}))}
	go srv.Serve(ln)
	defer srv.Close()
	addr := ln.Addr().String()

	// HTTPS reaches the handler, over HTTP/2.
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true}
	resp, err := (&http.Client{Transport: tr}).Get("https://" + addr + "/x")
	if err != nil {
		t.Fatalf("https: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "secure /x" || resp.ProtoMajor != 2 {
		t.Fatalf("https got %q over %s", body, resp.Proto)
	}

	// Plain HTTP on the same port is redirected to HTTPS.
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err = noFollow.Get("http://" + addr + "/login?next=%2F")
	if err != nil {
		t.Fatalf("http: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("http status = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://"+addr+"/login?next=%2F" {
		t.Fatalf("Location = %q", loc)
	}

	// A connection that closes before sending anything does not stop the listener.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	resp, err = (&http.Client{Transport: tr}).Get("https://" + addr + "/again")
	if err != nil || resp == nil {
		t.Fatalf("https after empty conn: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasSuffix(string(body), "/again") {
		t.Fatalf("got %q", body)
	}
}

func TestDualListenerCloseUnblocksAccept(t *testing.T) {
	cfg, _, _, err := GenerateSelfSigned("localhost")
	if err != nil {
		t.Fatal(err)
	}
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := NewDualListener(inner, cfg)
	done := make(chan error, 1)
	go func() { _, err := ln.Accept(); done <- err }()
	ln.Close()
	if err := <-done; err == nil {
		t.Fatal("Accept returned no error after Close")
	}
}
