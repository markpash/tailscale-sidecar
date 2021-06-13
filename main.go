package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"tailscale.com/tsnet"
)

type Binding struct {
	From uint16 `json:"from"`
	To   string `json:"to"`
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
		defaultDir := "./tsstate"
		if _, err := os.Stat(defaultDir); os.IsNotExist(err) {
			if err := os.Mkdir(defaultDir, 0755); err != nil {
				panic("failed to create default state directory")
			}
		}

		stateDir = defaultDir
	}

	return tsnet.Server{
		Dir:      stateDir,
		Hostname: hostname,
	}
}

func proxyBind(s tsnet.Server, b Binding) {
	ln, err := s.Listen("tcp", fmt.Sprintf(":%d", b.From))
	if err != nil {
		log.Println(err)
		return
	}

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
			proxyBind(s, binding)
		}(binding)
	}
	wg.Wait()
}
