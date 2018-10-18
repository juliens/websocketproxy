package websocketproxy

import (
	"net/http"
	"strings"
)

// Headers
const (
	XForwardedProto        = "X-Forwarded-Proto"
	XForwardedFor          = "X-Forwarded-For"
	XForwardedHost         = "X-Forwarded-Host"
	XForwardedPort         = "X-Forwarded-Port"
	XForwardedServer       = "X-Forwarded-Server"
	XRealIP                = "X-Real-Ip"
	Connection             = "Connection"
	KeepAlive              = "Keep-Alive"
	ProxyAuthenticate      = "Proxy-Authenticate"
	ProxyAuthorization     = "Proxy-Authorization"
	Te                     = "Te" // canonicalized version of "TE"
	Trailers               = "Trailers"
	TransferEncoding       = "Transfer-Encoding"
	Upgrade                = "Upgrade"
	ContentLength          = "Content-Length"
	SecWebsocketKey        = "Sec-Websocket-Key"
	SecWebsocketVersion    = "Sec-Websocket-Version"
	SecWebsocketExtensions = "Sec-Websocket-Extensions"
	SecWebsocketAccept     = "Sec-Websocket-Accept"
)

var hopHeaders = []string{
	Connection,
	KeepAlive,
	ProxyAuthenticate,
	ProxyAuthorization,
	Te, // canonicalized version of "TE"
	Trailers,
	TransferEncoding,
	Upgrade,
	SecWebsocketAccept,
	SecWebsocketExtensions,
	SecWebsocketKey,
	SecWebsocketVersion,
}

// WebsocketDialHeaders Websocket dial headers
var WebsocketDialHeaders = []string{
	Upgrade,
	Connection,
	SecWebsocketKey,
	SecWebsocketVersion,
	SecWebsocketExtensions,
	SecWebsocketAccept,
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// removeConnectionHeaders removes hop-by-hop headers listed in the "Connection" header of h.
// See RFC 7230, section 6.1
func removeConnectionHeaders(h http.Header) {
	if c := h.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				h.Del(f)
			}
		}
	}
}
