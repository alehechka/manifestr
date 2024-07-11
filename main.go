package main

import (
	"log"
	"manifestr/cmd"
	"os"
)

// Version of application
var Version = "dev"

func main() {
	if err := cmd.App(Version).Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
