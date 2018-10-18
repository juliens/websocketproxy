package websocketproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func TestWebSocketEcho(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	uri, err := url.ParseRequestURI(srv.URL)
	require.NoError(t, err)

	mux.Handle("/ws", websocket.Handler(func(conn *websocket.Conn) {
		msg := make([]byte, 4)
		msgLen, _ := conn.Read(msg)
		_, _ = conn.Write(msg[:msgLen])
		_ = conn.Close()
	}))

	p := NewSingleHostReverseProxy(uri)
	proxy := httptest.NewServer(p)
	defer proxy.Close()

	headers := http.Header{}
	serverAddr := proxy.Listener.Addr().String()
	webSocketURL := "ws://" + serverAddr + "/ws"
	headers.Add("Origin", webSocketURL)

	conn, resp, err := gorillawebsocket.DefaultDialer.Dial(webSocketURL, headers)
	require.NoError(t, err, "Error during Dial with response: %+v", resp)

	err = conn.WriteMessage(gorillawebsocket.TextMessage, []byte("OK"))
	require.NoError(t, err)

	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	err = conn.Close()
	require.NoError(t, err)
}
