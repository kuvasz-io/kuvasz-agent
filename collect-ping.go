package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

func request(service string, url string, headers []Header) {
	dialerror := false

	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: func(network string, address string) (net.Conn, error) {
			var dialer = &net.Dialer{
				Timeout: 2 * time.Second,
			}

			start := time.Now()
			conn, err := dialer.Dial(network, address)
			duration := float32(time.Since(start).Microseconds()) / 1000

			if err != nil {
				log.Error(3, "[PING] [%s] Cannot open http connection to %s, err=%v, time=%f", service, url, err.Error(), duration)
				send_now("ping."+service+".error.req", duration)
				dialerror = true
			} else {
				log.Debug("[PING] [%s] Connect to %s, time=%f", service, address, duration)
				send_now("ping."+service+".connect", duration)
			}
			return conn, err
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          1,
		IdleConnTimeout:       1 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	log.Trace("[PING] [%s] Create client", service)
	client := http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error(3, "[PING] [%s] Cannot create request for url=%s, error=%v", service, url, err.Error())
		return
	}
	req.Header.Set("User-agent", "kuvasz-pinger/1.1")
	for i := range headers {
		req.Header.Set(headers[i].key, headers[i].value)
	}
	start := time.Now()
	log.Debug("[PING] [%s] Open connection to: %s", service, url)
	resp, err := client.Do(req)
	if dialerror {
		return
	}
	duration := float32(time.Since(start).Microseconds()) / 1000
	if err != nil {
		log.Error(3, "[PING] [%s] Cannot open http connection to %s, err=%v, time=%f", service, url, err.Error(), duration)
		send_now("ping."+service+".error.req", duration)
		return
	}
	send_now("ping."+service+".request", duration)
	log.Debug("[PING] [%s] Request sent, time=%f", service, duration)

	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)

	duration = float32(time.Since(start).Microseconds()) / 1000
	if err != nil {
		log.Error(3, "[PING] [%s] Cannot read response: %s", service, err)
		send_now("ping."+service+".error.resp", duration)
		return
	}
	send_now(fmt.Sprintf("ping."+service+".total.%03d", resp.StatusCode), duration)
	log.Debug("[PING] [%s] Response received, time=%f", service, duration)
}

func CollectPing(service string, url string, headers []Header) {
	c := time.Tick(time.Duration(DELTA) * time.Second)
	log.Debug("[PING] [%s] Starting pinger with headers=%v", service, headers)
	for range c {
		request(service, url, headers)
	}
}
