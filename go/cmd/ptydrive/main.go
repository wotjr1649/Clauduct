//go:build windows

// ptydrive runs one program inside a Windows pseudo console (ConPTY) and types a scripted
// sequence into it. A maintainer tool: it is not a release asset. The driver is
// internal/ptydrive.
//
//	go run ./cmd/ptydrive -dir <cwd> -script steps.json -log raw.log -text screen.txt -env K=V ... -- <exe> <args...>
package main

import "github.com/wotjr1649/Clauduct/go/internal/ptydrive"

func main() { ptydrive.Main() }
