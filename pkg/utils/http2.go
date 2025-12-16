package utils

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// 跨语言GRPC客户端统一使用HTTP/2
func NewH2CClient(timeout time.Duration, transportCfgUpdates ...func(*http2.Transport)) *http.Client {
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	for _, fn := range transportCfgUpdates {
		fn(transport)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
