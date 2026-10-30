//go:build windows

package main

import (
	"os"

	"github.com/wotjr1649/Clauduct/go/internal/ptydrive"
)

func main() { os.Exit(ptydrive.Run(os.Args[1:], nil)) }
