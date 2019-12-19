package http

import (
	"strconv"
	"time"
)

const DateFormat = "Mon, Jan 02 2006 15:04:05 GMT"

type Bundle map[string]File

type File struct {
	Headers      map[string]string
	Content      []byte
	LastModified time.Time
}

var UIBundle Bundle

func (b Bundle) init() {
	for _, file := range b {
		file.Headers["content-length"] = strconv.Itoa(len(file.Content))
		lastModified, err := time.Parse(DateFormat, file.Headers["last-modified"])
		if err != nil {
			panic(err)
		}

		file.LastModified = lastModified
	}
}
