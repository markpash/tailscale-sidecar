package main

import (
	"io"
	"log"
	"net"
	"os"
	"sync"

	"tailscale.com/tsnet"
)

func main() {
	err := os.Setenv("TAILSCALE_USE_WIP_CODE", "true")
	if err != nil {
		panic(err)
	}

	s := tsnet.Server{
		Dir:      "./data",
		Hostname: "test-sidecar",
	}
	ln, err := s.Listen("tcp", ":80")
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		go func(left net.Conn) {
			defer left.Close()

			right, err := net.Dial("tcp", "127.0.0.1:8000")
			if err != nil {
				log.Println(err)
				return
			}
			defer right.Close()

			proxyConn := func(a, b net.Conn) {
				_, err := io.Copy(a, b)
				if err != nil {
					log.Println(err)
					return
				}
			}

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				proxyConn(right, left)
			}()

			go func() {
				defer wg.Done()
				proxyConn(left, right)
			}()

			wg.Wait()
		}(conn)
	}
}
