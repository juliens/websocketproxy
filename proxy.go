// Package websocketproxy FIXME
package websocketproxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/gorilla/websocket"
)

type logger interface {
	Printf(format string, args ...interface{})
}

// Dialer the websocket dialer
type Dialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

// NewSingleHostReverseProxy Creates a new ReverseProxy.
func NewSingleHostReverseProxy(target *url.URL) *ReverseProxy {
	targetQuery := target.RawQuery

	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)

		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}

		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}

		switch req.URL.Scheme {
		case "https":
			req.URL.Scheme = "wss"
		case "http":
			req.URL.Scheme = "ws"
		}
	}

	return &ReverseProxy{Director: director}
}

// ReverseProxy is an HTTP Handler that takes an incoming request and
// sends it to another server, proxying the response back to the
// client.
type ReverseProxy struct {
	// Director must be a function which modifies
	// the request into a new request to be sent
	// using Transport. Its response is then copied
	// back to the original client unmodified.
	// Director must not access the provided Request
	// after returning.
	Director func(*http.Request)

	// The dialer used to perform dial.
	// If nil, websocket.DefaultDialer is used.
	Dialer Dialer

	WebsocketConnectionClosedHook func(req *http.Request, conn net.Conn)

	ErrorHandler func(rw http.ResponseWriter, req *http.Request, err error)
	Logger       logger
}

func (p *ReverseProxy) logf(format string, args ...interface{}) {
	if p.Logger == nil {
		log.Printf(format, args...)
	}
	p.Logger.Printf(format, args...)
}

func (p *ReverseProxy) defaultErrorHandler(rw http.ResponseWriter, req *http.Request, err error) {
	p.logf("http: proxy error: %v", err)
	rw.WriteHeader(http.StatusBadGateway)
}

func (p *ReverseProxy) getErrorHandler() func(http.ResponseWriter, *http.Request, error) {
	if p.ErrorHandler != nil {
		return p.ErrorHandler
	}
	return p.defaultErrorHandler
}

func (p *ReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	dialer := p.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}

	outReq := new(http.Request)
	*outReq = *req

	outReq.Header = make(http.Header)
	copyHeader(outReq.Header, req.Header)

	p.Director(outReq)

	for _, h := range WebsocketDialHeaders {
		hv := outReq.Header.Get(h)
		if hv == "" {
			continue
		}
		outReq.Header.Del(h)
	}

	targetConn, resp, err := dialer.DialContext(outReq.Context(), outReq.URL.String(), outReq.Header)
	if err != nil {
		if resp == nil {
			p.logf("websocket: Error dialing %q: %v", req.Host, err)
			p.getErrorHandler()(rw, outReq, err)
			return
		}

		p.logf("websocket: Error dialing %q: %v with resp: %d %s", req.Host, err, resp.StatusCode, resp.Status)
		hijacker, ok := rw.(http.Hijacker)
		if !ok {
			p.logf("websocket: %s can not be hijack", reflect.TypeOf(rw))
			p.getErrorHandler()(rw, outReq, err)
			return
		}

		conn, _, errHijack := hijacker.Hijack()
		if errHijack != nil {
			p.logf("websocket: Failed to hijack responseWriter")
			p.getErrorHandler()(rw, outReq, errHijack)
			return
		}
		defer func() { _ = conn.Close() }()

		errWrite := resp.Write(conn)
		if errWrite != nil {
			p.logf("websocket: Failed to forward response")
			p.getErrorHandler()(rw, outReq, errWrite)
			return
		}
		return
	}

	// Only the targetConn choose to CheckOrigin or not
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return true
	}}

	removeConnectionHeaders(resp.Header)
	removeHopHeaders(resp.Header)

	copyHeader(resp.Header, rw.Header())

	underlyingConn, err := upgrader.Upgrade(rw, req, resp.Header)
	if err != nil {
		p.logf("websocket: Error while upgrading connection : %v", err)
		return
	}

	defer func() {
		_ = underlyingConn.Close()
		_ = targetConn.Close()
		if p.WebsocketConnectionClosedHook != nil {
			p.WebsocketConnectionClosedHook(req, underlyingConn.UnderlyingConn())
		}
	}()

	errClient := make(chan error, 1)
	errBackend := make(chan error, 1)

	go replicateWebsocketConn(underlyingConn, targetConn, errClient)
	go replicateWebsocketConn(targetConn, underlyingConn, errBackend)

	var message string
	select {
	case err = <-errClient:
		message = "websocket: Error when copying from backend to client: %v"
	case err = <-errBackend:
		message = "websocket: Error when copying from client to backend: %v"

	}
	if e, ok := err.(*websocket.CloseError); !ok || e.Code == websocket.CloseAbnormalClosure {
		p.logf(message, err)
	}
}

func replicateWebsocketConn(dst, src *websocket.Conn, errc chan error) {
	forward := func(messageType int, reader io.Reader) error {
		writer, err := dst.NextWriter(messageType)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, reader)
		if err != nil {
			return err
		}
		return writer.Close()
	}

	src.SetPingHandler(func(data string) error {
		return forward(websocket.PingMessage, bytes.NewReader([]byte(data)))
	})

	src.SetPongHandler(func(data string) error {
		return forward(websocket.PongMessage, bytes.NewReader([]byte(data)))
	})

	for {
		msgType, reader, err := src.NextReader()

		if err != nil {
			m := websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("%v", err))
			if e, ok := err.(*websocket.CloseError); ok {
				if e.Code != websocket.CloseNoStatusReceived {
					m = nil
					// Following codes are not valid on the wire so just close the
					// underlying TCP connection without sending a close frame.
					if e.Code != websocket.CloseAbnormalClosure &&
						e.Code != websocket.CloseTLSHandshake {

						m = websocket.FormatCloseMessage(e.Code, e.Text)
					}
				}
			}
			errc <- err
			if m != nil {
				// FIXME manage error?
				_ = forward(websocket.CloseMessage, bytes.NewReader(m))
			}
			break
		}
		err = forward(msgType, reader)
		if err != nil {
			errc <- err
			break
		}
	}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
