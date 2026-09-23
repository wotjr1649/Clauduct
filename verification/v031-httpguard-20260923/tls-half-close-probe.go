package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"
)

// The private key stays in memory. Trust is limited to this diagnostic client;
// no certificate store, filtering configuration or system setting is changed.
func main() {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { panic(err) }
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil { panic(err) }
	cert, err := x509.ParseCertificate(der)
	if err != nil { panic(err) }
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	serverConfig := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	clientConfig := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "127.0.0.1"}
	for _, rawFIN := range []bool{false, true} {
		for attempt := 0; attempt < 3; attempt++ {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil { panic(err) }
			done := make(chan string, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil { done <- "accept_error"; return }
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				server := tls.Server(conn, serverConfig)
				input := make([]byte, 6)
				n, readErr := io.ReadFull(server, input)
				time.Sleep(50 * time.Millisecond)
				written, writeErr := io.WriteString(server, "PUBLIC_REPLY")
				endErr := server.CloseWrite()
				time.Sleep(100 * time.Millisecond)
				done <- fmt.Sprintf("request_bytes=%d request_error=%v response_bytes=%d response_error=%v close_notify_error=%v", n, readErr, written, writeErr, endErr)
			}()
			conn, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
			if err != nil { panic(err) }
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			client := tls.Client(conn, clientConfig)
			if err := client.Handshake(); err != nil { panic(err) }
			_, writeErr := io.WriteString(client, "PUBLIC")
			var endErr error
			if rawFIN { endErr = conn.(*net.TCPConn).CloseWrite() } else { endErr = client.CloseWrite() }
			body, readErr := io.ReadAll(client)
			serverResult := <-done
			conn.Close()
			listener.Close()
			fmt.Printf("raw_tcp_fin=%t attempt=%d client_write_error=%v half_close_error=%v reply_bytes=%d marker=%t read_error=%v %s\n", rawFIN, attempt, writeErr, endErr, len(body), string(body) == "PUBLIC_REPLY", readErr, serverResult)
		}
	}
}
