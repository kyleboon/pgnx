package main

import "os"

func main() {
	os.Exit(run())
}

func run() int {
	root := NewRootCmd()
	err := root.Execute()
	return exitCode(err)
}
