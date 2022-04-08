// The content of this is copied from
// github.com/tailscale/tailscale/commit/7c7f37342fa710bbf223aa29c5631f2d182fa59d
// with the following license.

// BSD 3-Clause License
//
// Copyright (c) 2020 Tailscale & AUTHORS. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice,
// this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"

	"tailscale.com/net/proxymux"
	"tailscale.com/net/socks5"
	"tailscale.com/tsnet"
	"tailscale.com/types/logger"
)

func runProxies(tsnetServer *tsnet.Server, socksAddr, httpProxyAddr string) error {
	socksListener, httpProxyListener := mustStartProxyListeners(socksAddr, httpProxyAddr)
	if socksListener == nil && httpProxyListener == nil {
		return errors.New("proxy listeners are nil")
	}
	defer socksListener.Close()
	defer httpProxyListener.Close()

	ec := make(chan error, 1)
	if httpProxyListener != nil {
		hs := &http.Server{Handler: httpProxyHandler(tsnetServer.Dial)}
		go func() {
			ec <- fmt.Errorf("http proxy exited with error: %w", hs.Serve(httpProxyListener))
		}()
	}

	if socksListener != nil {
		ss := &socks5.Server{
			Logf:   logger.WithPrefix(log.Printf, "socks5: "),
			Dialer: tsnetServer.Dial,
		}

		go func() {
			ec <- fmt.Errorf("socks5 server exited with error: %w", ss.Serve(socksListener))
		}()
	}

	return <-ec
}

// copied from: cmd/tailscaled/tailscaled.go
//
// mustStartProxyListeners creates listeners for local SOCKS and HTTP
// proxies, if the respective addresses are not empty. socksAddr and
// httpAddr can be the same, in which case socksListener will receive
// connections that look like they're speaking SOCKS and httpListener
// will receive everything else.
//
// socksListener and httpListener can be nil, if their respective
// addrs are empty.
func mustStartProxyListeners(socksAddr, httpAddr string) (socksListener, httpListener net.Listener) {
	if socksAddr == httpAddr && socksAddr != "" && !strings.HasSuffix(socksAddr, ":0") {
		ln, err := net.Listen("tcp", socksAddr)
		if err != nil {
			log.Fatalf("proxy listener: %v", err)
		}
		return proxymux.SplitSOCKSAndHTTP(ln)
	}

	var err error
	if socksAddr != "" {
		socksListener, err = net.Listen("tcp", socksAddr)
		if err != nil {
			log.Fatalf("SOCKS5 listener: %v", err)
		}
		if strings.HasSuffix(socksAddr, ":0") {
			// Log kernel-selected port number so integration tests
			// can find it portably.
			log.Printf("SOCKS5 listening on %v", socksListener.Addr())
		}
	}
	if httpAddr != "" {
		httpListener, err = net.Listen("tcp", httpAddr)
		if err != nil {
			log.Fatalf("HTTP proxy listener: %v", err)
		}
		if strings.HasSuffix(httpAddr, ":0") {
			// Log kernel-selected port number so integration tests
			// can find it portably.
			log.Printf("HTTP proxy listening on %v", httpListener.Addr())
		}
	}

	return socksListener, httpListener
}

// copied from: cmd/tailscaled/proxy.go
//
// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP proxy code

// httpProxyHandler returns an HTTP proxy http.Handler using the
// provided backend dialer.
func httpProxyHandler(dialer func(ctx context.Context, netw, addr string) (net.Conn, error)) http.Handler {
	rp := &httputil.ReverseProxy{
		Director: func(r *http.Request) {}, // no change
		Transport: &http.Transport{
			DialContext: dialer,
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			backURL := r.RequestURI
			if strings.HasPrefix(backURL, "/") || backURL == "*" {
				http.Error(w, "bogus RequestURI; must be absolute URL or CONNECT", 400)
				return
			}
			rp.ServeHTTP(w, r)
			return
		}

		// CONNECT support:

		dst := r.RequestURI
		c, err := dialer(r.Context(), "tcp", dst)
		if err != nil {
			w.Header().Set("Tailscale-Connect-Error", err.Error())
			http.Error(w, err.Error(), 500)
			return
		}
		defer c.Close()

		cc, ccbuf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer cc.Close()

		io.WriteString(cc, "HTTP/1.1 200 OK\r\n\r\n")

		var clientSrc io.Reader = ccbuf
		if ccbuf.Reader.Buffered() == 0 {
			// In the common case (with no
			// buffered data), read directly from
			// the underlying client connection to
			// save some memory, letting the
			// bufio.Reader/Writer get GC'ed.
			clientSrc = cc
		}

		errc := make(chan error, 1)
		go func() {
			_, err := io.Copy(cc, c)
			errc <- err
		}()
		go func() {
			_, err := io.Copy(c, clientSrc)
			errc <- err
		}()
		<-errc
	})
}
