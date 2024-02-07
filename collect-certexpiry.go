package main

import (
	"crypto/tls"
	"net"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

func CollectCertExpiry(site string, domain string) {
	var m Metrics
	d := net.Dialer{Timeout: 5 * time.Second}

	for {
		conn, err := d.Dial("tcp", domain)
		if err != nil {
			log.Error(3, "[CERT] Cannot open connection to %s: %v", domain, err)
			time.Sleep(1 * time.Minute)
			continue
		}

		config := &tls.Config{
			InsecureSkipVerify: true, // allows self-signed or expired certificates
			ServerName:         strings.Split(domain, ":")[0],
		}

		connTLS := tls.Client(conn, config)
		if err := connTLS.Handshake(); err != nil {
			log.Error(3, "[CERT] Cannot handshake with %s: %v", domain, err)
			conn.Close()
			time.Sleep(1 * time.Minute)
			continue
		}
		days := time.Until(connTLS.ConnectionState().PeerCertificates[0].NotAfter).Hours() / 24
		log.Debug("[CERT] %s: %v", domain, days)
		m = send_metric(m, "cert."+site, float32(days))
		metricschannel <- m
		m = nil
		conn.Close()
		time.Sleep(1 * time.Hour)
	}
}
