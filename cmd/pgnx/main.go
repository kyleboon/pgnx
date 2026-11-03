package main

import "os"

func main() {
	os.Exit(execute(NewRootCmd(), os.Args[1:]))
}
