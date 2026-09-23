package main

import (
	"fmt"
	"io"
	"net"
	"strings"
"syscall"
	"time"
)

func main() {
	for _, target := range []struct{ network, address string }{{"tcp4", "127.0.0.1:0"}, {"tcp6", "[::1]:0"}} {
		for _, delay := range []time.Duration{0, 10 * time.Millisecond} {
			for attempt := 0; attempt < 3; attempt++ {
				listener, err := net.Listen(target.network, target.address)
				if err != nil {
					fmt.Printf("network=%s listen_error=%T\n", target.network, err)
					break
				}
				request := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", listener.Addr())
				accepted := make(chan struct{})
				done := make(chan string, 1)
				go func() {
					c, err := listener.Accept()
					if err != nil {
						close(accepted)
						done <- "accept_error"
						return
					}
					defer c.Close()
					c.SetDeadline(time.Now().Add(time.Second))
					close(accepted)
					input := make([]byte, len(request))
					n, readErr := io.ReadFull(c, input)
					written, writeErr := io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 12\r\nConnection: close\r\n\r\nPUBLIC_REPLY")
					time.Sleep(500 * time.Millisecond)
					done <- fmt.Sprintf("server_read=%d/%d server_read_error=%v server_write=%d server_write_error=%v", n, len(request), readErr, written, writeErr)
				}()
				c, err := net.DialTimeout(target.network, listener.Addr().String(), time.Second)
				if err != nil {
					listener.Close()
					<-done
					fmt.Printf("network=%s dial_error=%T\n", target.network, err)
					continue
				}
				<-accepted
				c.SetDeadline(time.Now().Add(2 * time.Second))
				_, writeErr := io.WriteString(c, request)
				time.Sleep(delay)
				shutdownErr := sendDisconnect(c.(*net.TCPConn))
				reply, readErr := io.ReadAll(c)
				c.Close()
				server := <-done
				listener.Close()
				fmt.Printf("network=%s fin_delay=%s attempt=%d client_write_error=%v shutdown_error=%v client_read_error=%v reply_bytes=%d marker=%t %s\n", target.network, delay, attempt, writeErr, shutdownErr, readErr, len(reply), strings.Contains(string(reply), "PUBLIC_REPLY"), server)
			}
		}
	}
}

func sendDisconnect(c *net.TCPConn) error {
 raw, err := c.SyscallConn()
 if err != nil { return err }
 proc := syscall.NewLazyDLL("ws2_32.dll").NewProc("WSASendDisconnect")
 var callErr error
 err = raw.Control(func(fd uintptr) { result, _, e := proc.Call(fd, 0); if int32(result) == -1 { callErr = e } })
 if err != nil { return err }
 return callErr
}