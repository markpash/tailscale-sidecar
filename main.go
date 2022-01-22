package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"

	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

type Binding struct {
	From     uint16       `json:"from"`
	To       string       `json:"to"`
	Tls      bool         `json:"tls"`
	Protocol string       `json:"protocol"`
	Http     *HttpBinding `json:"http"`
}

type HttpBinding struct {
	Host    string            `json:"host"`
	Headers map[string]string `json:"headers"`
}

func loadBindings() ([]Binding, error) {
	bindingsPath := os.Getenv("TS_SIDECAR_BINDINGS")
	if bindingsPath == "" {
		bindingsPath = "/etc/ts-sidecar/bindings.json"
	}

	bindingsFile, err := os.Open(bindingsPath)
	if err != nil {
		return nil, err
	}
	defer bindingsFile.Close()

	d := json.NewDecoder(bindingsFile)

	var bindings []Binding
	if err := d.Decode(&bindings); err != nil {
		return nil, err
	}

	if len(bindings) == 0 {
		return nil, errors.New("bindings empty")
	}

	return bindings, nil
}

func newTsNetServer() tsnet.Server {
	hostname := os.Getenv("TS_SIDECAR_NAME")
	if hostname == "" {
		panic("TS_SIDECAR_NAME env var not set")
	}

	stateDir := os.Getenv("TS_SIDECAR_STATEDIR")
	if stateDir == "" {
		stateDir = "./tsstate"
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		panic("failed to create default state directory")
	}

	return tsnet.Server{
		Dir:      stateDir,
		Hostname: hostname,
	}
}

func proxyBind(s *tsnet.Server, b *Binding) {
	ln, err := s.Listen("tcp", fmt.Sprintf(":%d", b.From))
	if err != nil {
		log.Println(err)
		return
	}

	if b.Tls {
		ln = tls.NewListener(ln, &tls.Config{
			GetCertificate: tailscale.GetCertificate,
		})
	}

	if b.Protocol == "http" {
		serveHTTP(b, ln)
	} else if b.Protocol == "tcp" || b.Protocol == "" {
		serveTCP(b, ln)
	} else {
		log.Printf("unknown protocol %q", b.Protocol)
	}

}

func serveHTTP(b *Binding, ln net.Listener) {
	uri, err := url.Parse(b.To)
	if err != nil {
		var err2 error
		uri, err2 = url.Parse("http://" + b.To)
		if err2 != nil {
			log.Println(err)
			return
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(uri)

	if b.Http != nil {
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)

			for k, v := range b.Http.Headers {
				req.Header.Set(k, v)
			}

			if b.Http.Host != "" {
				req.Host = b.Http.Host
			}
		}
	}

	log.Printf("started proxy bind from %d to %v (tls: %t)", b.From, uri.String(), b.Tls)

	mux := http.NewServeMux()

	// handle all requests to your server using the proxy
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	err = http.Serve(ln, mux)
	if err != nil {
		log.Println(err)
	}
}

func serveTCP(b *Binding, ln net.Listener) {
	log.Printf("started proxy bind from %d to %v (tls: %t)", b.From, b.To, b.Tls)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		go func(left net.Conn) {
			defer left.Close()

			right, err := net.Dial("tcp", b.To)
			if err != nil {
				log.Println(err)
				return
			}
			defer right.Close()

			var wg sync.WaitGroup
			proxyConn := func(a, b net.Conn) {
				defer wg.Done()
				_, err := io.Copy(a, b)
				if err != nil {
					log.Println(err)
					return
				}
			}

			wg.Add(2)
			go proxyConn(right, left)
			go proxyConn(left, right)

			wg.Wait()
		}(conn)
	}
}

func main() {
	// Apparently this envvar needs to be set for this to work!
	err := os.Setenv("TAILSCALE_USE_WIP_CODE", "true")
	if err != nil {
		panic(err)
	}

	bindings, err := loadBindings()
	if err != nil {
		panic(err)
	}

	s := newTsNetServer()

	var wg sync.WaitGroup
	for _, binding := range bindings {
		wg.Add(1)
		go func(binding Binding) {
			defer wg.Done()
			proxyBind(&s, &binding)
		}(binding)
	}
	wg.Wait()
}
