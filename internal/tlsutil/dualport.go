package tlsutil

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"
)

// sniffTimeout bounds how long a new connection may take to send its first
// byte before it is dropped, so idle clients cannot hold a slot.
const sniffTimeout = 10 * time.Second

// DualListener serves TLS and plain connections on one port. Connections
// that start with a TLS handshake are returned as *tls.Conn; the rest are
// returned as they are, so an http.Server can answer them with a redirect
// (see RedirectPlainHTTP).
type DualListener struct {
	inner net.Listener
	cfg   *tls.Config
	conns chan net.Conn

	mu     sync.Mutex
	err    error
	closed chan struct{}
	once   sync.Once
}

// NewDualListener wraps inner. cfg is cloned and offered HTTP/2 and HTTP/1.1.
func NewDualListener(inner net.Listener, cfg *tls.Config) *DualListener {
	c := cfg.Clone()
	for _, p := range []string{"h2", "http/1.1"} {
		if !slices.Contains(c.NextProtos, p) {
			c.NextProtos = append(c.NextProtos, p)
		}
	}
	l := &DualListener{inner: inner, cfg: c, conns: make(chan net.Conn), closed: make(chan struct{})}
	go l.acceptLoop()
	return l
}

func (l *DualListener) acceptLoop() {
	for {
		c, err := l.inner.Accept()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			l.mu.Lock()
			l.err = err
			l.mu.Unlock()
			l.Close()
			return
		}
		go l.classify(c)
	}
}

// classify peeks at the first byte: 0x16 starts a TLS handshake record.
func (l *DualListener) classify(c net.Conn) {
	_ = c.SetReadDeadline(time.Now().Add(sniffTimeout))
	br := bufio.NewReader(c)
	first, err := br.Peek(1)
	_ = c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return
	}
	var out net.Conn = &peekedConn{Conn: c, r: br}
	if first[0] == 0x16 {
		out = tls.Server(out, l.cfg)
	}
	select {
	case l.conns <- out:
	case <-l.closed:
		out.Close()
	}
}

// Accept returns the next classified connection.
func (l *DualListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.err != nil {
			return nil, l.err
		}
		return nil, net.ErrClosed
	}
}

// Close stops accepting connections.
func (l *DualListener) Close() error {
	var err error
	l.once.Do(func() {
		close(l.closed)
		err = l.inner.Close()
	})
	return err
}

// TLSConfig returns the configuration used for TLS connections. Set it as
// the http.Server's TLSConfig so the server enables HTTP/2 as offered.
func (l *DualListener) TLSConfig() *tls.Config { return l.cfg }

// Addr returns the listener's network address.
func (l *DualListener) Addr() net.Addr { return l.inner.Addr() }

// peekedConn replays the bytes read while classifying the connection.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// RedirectPlainHTTP sends requests that arrived without TLS to the same host
// and path over HTTPS, and passes TLS requests to next.
func RedirectPlainHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			next.ServeHTTP(w, r)
			return
		}
		if r.Host == "" {
			http.Error(w, "use https", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}
