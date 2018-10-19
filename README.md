# websocketproxy

[![GoDoc](https://godoc.org/github.com/juliens/websocketproxy?status.svg)](https://godoc.org/github.com/juliens/websocketproxy)
[![Build Status](https://travis-ci.org/juliens/websocketproxy.svg?branch=master)](https://travis-ci.org/juliens/websocketproxy)
[![Go Report Card](https://goreportcard.com/badge/juliens/websocketproxy)](http://goreportcard.com/report/juliens/websocketproxy)

## Example

```go
proxy := NewSingleHostReverseProxy(uri)
server := httptest.NewServer(proxy)
defer server.Close()
```