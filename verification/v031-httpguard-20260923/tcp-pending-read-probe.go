package main

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

func main() {
	for _, pending := range []bool{false, true} {
		for attempt := 0; attempt < 3; attempt++ {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil { panic(err) }
			done := make(chan string, 1)
			go func() {
				c, err := listener.Accept()
				if err != nil { done <- "accept_error"; return }
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				input := make([]byte, 6)
				n, readErr := io.ReadFull(c, input)
				time.Sleep(50 * time.Millisecond)
				written, writeErr := io.WriteString(c, "PUBLIC_REPLY")
				time.Sleep(100 * time.Millisecond)
				done <- fmt.Sprintf("request_bytes=%d request_error=%v response_bytes=%d response_error=%v", n, readErr, written, writeErr)
			}()
			c, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
			if err != nil { panic(err) }
			c.SetDeadline(time.Now().Add(time.Second))
			readDone := make(chan string, 1)
			read := func() {
				body, firstErr := io.ReadAll(c)
				time.Sleep(100 * time.Millisecond)
				rest, secondErr := io.ReadAll(c)
				readDone <- fmt.Sprintf("reply_bytes=%d marker=%t first_error=%v second_bytes=%d second_error=%v", len(body), strings.Contains(string(body), "PUBLIC_REPLY"), firstErr, len(rest), secondErr)
			}
			if pending { go read(); time.Sleep(5 * time.Millisecond) }
			_, writeErr := io.WriteString(c, "PUBLIC")
			closeErr := c.(*net.TCPConn).CloseWrite()
			if !pending { go read() }
			readResult := <-readDone
			serverResult := <-done
			c.Close()
			listener.Close()
			fmt.Printf("pending_read=%t attempt=%d write_error=%v half_close_error=%v %s %s\n", pending, attempt, writeErr, closeErr, readResult, serverResult)
		}
	}
}
