package main

import (
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/config"
	"os"
)

func main() {
	file, err := os.Open(os.Args[1])

	if err != nil {
		panic(err)
	}

	conf, err := config.ParseConfig(file)

	if err != nil {
		panic(err)
	}

	fmt.Println(conf.String())
}
