package main

import (
	"fmt"
	"os"

	"bdtui"
)

func main() {
	if err := bdtui.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
