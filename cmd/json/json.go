package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	clientConfig := struct {
		Bits    int  `json:"bits"`
		Dynamic bool `json:"dynamic"`
	}{4096, true}

	bs, err := json.Marshal(clientConfig)

	if err != nil {
		panic(err)
	}

	text := string(bs)

	fmt.Println(text)
}
