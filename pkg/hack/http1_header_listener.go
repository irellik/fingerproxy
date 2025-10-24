package hack

import (
	"errors"
	"io"
	"net"
	"syscall"
)

// HTTP1HeaderListener wraps an existing listener and returns
// connections that record HTTP/1.x header order.
type HTTP1HeaderListener struct{ net.Listener }

func NewHTTP1HeaderListener(inner net.Listener) *HTTP1HeaderListener {
	return &HTTP1HeaderListener{inner}
}

// Accept waits for and returns the next connection to the listener. It wraps
// accepted connections with HTTP1HeaderConn to capture the order of HTTP/1.x
// headers. If the client closes the connection before sending a complete
// request (resulting in io.EOF or io.ErrUnexpectedEOF) or times out while
// sending headers, the connection is discarded and Accept continues to wait for
// the next one instead of returning an error up the stack which would stop the
// HTTP server.
func (l *HTTP1HeaderListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		hc, err := NewHTTP1HeaderConn(c)
		if err != nil {
			// Must close the underlying TLSClientHelloConn properly
			// to trigger Done() and prevent goroutine leak
			if tlsConn, ok := c.(*TLSClientHelloConn); ok {
				tlsConn.Close() // This calls Done() internally
			} else {
				c.Close()
			}
			// Ignore all network and client errors - these should not bring down the HTTP server.
			// Only propagate errors that indicate a serious problem with the listener itself.
			var ne net.Error
			if errors.Is(err, io.EOF) ||
				errors.Is(err, io.ErrUnexpectedEOF) ||
				errors.Is(err, syscall.ECONNRESET) ||
				errors.Is(err, syscall.EPIPE) ||
				errors.Is(err, syscall.ECONNABORTED) ||
				(errors.As(err, &ne) && (ne.Timeout() || ne.Temporary())) {
				continue
			}
			// For any other network operation error, also continue
			var opErr *net.OpError
			if errors.As(err, &opErr) {
				continue
			}
			return nil, err
		}
		return hc, nil
	}
}
