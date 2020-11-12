package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"go/format"
	"io/ioutil"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/amadigan/openvpn-aws/internal/web"

	"github.com/google/brotli/go/cbrotli"
)

var brotliOptions cbrotli.WriterOptions = cbrotli.WriterOptions{
	Quality: 11,
	LGWin:   24,
}

var immutableExtensions map[string]bool = map[string]bool{
	".js":    true,
	".woff2": true,
}

type bundleContext struct {
	root         string
	lastModified string
}

func main() {
	if mime.TypeByExtension(".md") == "" {
		mime.AddExtensionType(".md", web.TypeMarkdown)
	}

	if mime.TypeByExtension(".ovpn") == "" {
		mime.AddExtensionType(".ovpn", "application/x-openvpn-profile")
	}

	if mime.TypeByExtension(".woff2") == "" {
		mime.AddExtensionType(".woff2", "font/woff2")
	}

	if len(os.Args) != 3 {
		fmt.Printf("Usage: %s directory file\n", os.Args[0])
		return
	}

	root, err := filepath.Abs(os.Args[1])

	if err != nil {
		panic(err)
	}

	context := bundleContext{
		root:         root,
		lastModified: time.Now().UTC().Format(web.DateFormat),
	}

	bundle := web.Bundle{}

	err = walk(context, bundle, root)

	writer := bytes.NewBuffer(nil)
	writer.WriteString("package web\n\n")
	writer.WriteString("func init() {\n")
	writer.WriteString("UIBundle = Bundle{\n")

	for key, value := range bundle {
		writer.WriteString(fmt.Sprintf("%#v : File {\n", key))
		writer.WriteString(fmt.Sprintf("Headers: %#v,\n", value.Headers))
		writer.WriteString("Content: []byte{")

		for _, b := range value.Content {
			writer.WriteString(fmt.Sprintf("%d, ", b))
		}

		writer.WriteString("},\n},\n")
	}

	writer.WriteString("}\n")

	writer.WriteString("UIBundle.init()\n}")

	if err != nil {
		panic(err)
	}

	formatted, err := format.Source(writer.Bytes())

	if err != nil {
		panic(err)
	}

	file, err := os.OpenFile(os.Args[2], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		panic(err)
	}

	defer file.Close()

	fmt.Printf("saving to %s\n", os.Args[2])

	_, err = file.Write(formatted)

	if err != nil {
		panic(err)
	}
}

func walk(context bundleContext, bundle web.Bundle, dir string) error {
	infos, err := ioutil.ReadDir(dir)

	if err != nil {
		return err
	}

	for _, info := range infos {
		if info.Mode().IsRegular() {
			key, file, err := readFile(filepath.Join(dir, info.Name()), context, info)

			if err != nil {
				return err
			}

			bundle[key] = file
		} else if info.IsDir() {
			walk(context, bundle, filepath.Join(dir, info.Name()))
		}
	}

	return nil
}

func readFile(path string, context bundleContext, info os.FileInfo) (key string, fileObj web.File, err error) {
	key = strings.TrimPrefix(path, context.root)

	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}

	if strings.HasSuffix(key, "/index.html") {
		key = strings.TrimSuffix(key, "index.html")
	}

	extension := filepath.Ext(path)

	contentType := mime.TypeByExtension(extension)

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	file, err := os.Open(path)

	if err != nil {
		return key, fileObj, err
	}

	defer file.Close()

	bs, err := ioutil.ReadAll(file)

	hashCode := sha256.Sum256(bs)
	hashString := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hashCode[:])
	digestString := base64.StdEncoding.EncodeToString(hashCode[:])

	fileObj.Headers = map[string]string{
		"content-type":  contentType,
		"etag":          hashString,
		"last-modified": context.lastModified,
		"digest":        "sha-256=" + digestString,
	}

	if immutableExtensions[extension] {
		fileObj.Headers["cache-control"] = "public,immutable,max-age=" + strconv.Itoa(60*60*24*365)
	}

	brBuffer := bytes.NewBuffer(make([]byte, 0, len(bs)))

	brWriter := cbrotli.NewWriter(brBuffer, brotliOptions)

	_, err = brWriter.Write(bs)

	if err != nil {
		return key, fileObj, err
	}

	err = brWriter.Close()

	if err != nil {
		return key, fileObj, err
	}

	gzBuffer := bytes.NewBuffer(make([]byte, 0, len(bs)))

	gzWriter, _ := gzip.NewWriterLevel(gzBuffer, gzip.BestCompression)

	_, err = gzWriter.Write(bs)

	if err != nil {
		return key, fileObj, err
	}

	err = gzWriter.Close()

	if err != nil {
		return key, fileObj, err
	}

	minSize := int(float32(len(bs))*.98 - 50)

	if brBuffer.Len() < gzBuffer.Len() && brBuffer.Len() < minSize {
		fmt.Printf("%s - br encoding %d -> %d (%2.2f%% reduction)\n", key, len(bs), brBuffer.Len(), (1-(float32(brBuffer.Len())/float32(len(bs))))*100)
		fileObj.Headers["content-encoding"] = "br"
		bs = brBuffer.Bytes()
	} else if gzBuffer.Len() < minSize {
		fmt.Printf("%s - gz encoding %d -> %d (%2.2f%% reduction)\n", key, len(bs), gzBuffer.Len(), (1-(float32(gzBuffer.Len())/float32(len(bs))))*100)
		fileObj.Headers["content-encoding"] = "gzip"
		bs = gzBuffer.Bytes()
	} else {
		fmt.Printf("%s - no encoding (%d bytes)\n", key, len(bs))
	}

	fmt.Printf("Headers: %#v\n", fileObj.Headers)

	fileObj.Content = bs
	return key, fileObj, nil
}
