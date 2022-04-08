package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/sync/errgroup"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

type Binding struct {
	From uint16 `json:"from"`
	To   string `json:"to"`
	Tls  bool   `json:"tls"`
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

	// if len(bindings) == 0 {
	// 	return nil, errors.New("bindings empty")
	// }

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

	proxyToTailnet := flag.Bool("proxy-to-tailnet", false, "EXPERIMENTAL: flag to enable proxying into the tailnet with socks and http proxies")
	socksAddr := flag.String("socksproxy", "localhost:1080", "set the address for socks proxy to listen on")
	httpProxyAddr := flag.String("httpproxy", "localhost:8080", "set the address for http proxy to listen on")
	flag.Parse()

	bindings, err := loadBindings()
	if err != nil {
		panic(err)
	}

	s := newTsNetServer()
	if err := s.Start(); err != nil {
		panic(err)
	}

	eg := errgroup.Group{}

	if *proxyToTailnet {
		eg.Go(func() error {
			return runProxies(&s, *socksAddr, *httpProxyAddr)
		})
	}

	for _, binding := range bindings {
		binding := binding
		eg.Go(func() error {
			proxyBind(&s, &binding)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		panic(err)
	}
}
