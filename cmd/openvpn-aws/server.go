package main

import (
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/ca"
	"github.com/amadigan/openvpn-aws/internal/config"
	"github.com/amadigan/openvpn-aws/internal/log"
	"github.com/amadigan/openvpn-aws/internal/vpn"
	"github.com/pborman/getopt/v2"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	log.LogLevel = log.DEBUG
	if len(os.Args) > 2 && os.Args[1] == "verify" {
		if os.Args[2] == "0" {
			os.Exit(handleVerify())
		} else {
			return
		}
	}

	handleStart()
}

func str(s string) *string {
	return &s
}

func handleStart() {
	s3path := str(os.Getenv("S3_PATH"))
	localPath := str("")

	if *s3path != "" {
		localPath = str("")
	}

	getopt.SetProgram(os.Args[0])
	getopt.SetParameters("")
	s3path = getopt.StringLong("s3", 's', *s3path, "S3 directory containing vpn.conf. May be an s3:// URL or bucket/path", "url")
	localPath = getopt.StringLong("local", 'l', *localPath, "Filesystem path containing vpn.conf", "path")
	root := getopt.StringLong("root", 'r', ".", "Root path for VPN", "path")
	logLevel := getopt.EnumLong("loglevel", 0, []string{"debug", "info", "warn", "error"}, "debug", "Log verbosity", "level")
	showHelp := getopt.BoolLong("help", 'h', "Show help")

	getopt.Parse()

	if *showHelp {
		help(0)
	}

	switch *logLevel {
	case "debug":
		log.LogLevel = log.DEBUG
		break
	case "info":
		log.LogLevel = log.INFO
		break
	case "warn":
		log.LogLevel = log.WARN
		break
	case "error":
		log.LogLevel = log.ERROR
		break
	}

	absRoot, err := filepath.Abs(*root)

	if err != nil {
		errorf("Unable to parse root path %s: %s", *root, err)
	}

	var backend config.ConfigurationBackend

	if *s3path != "" {
		s3 := *s3path
		var bucket string
		var path string

		if strings.HasPrefix(s3, "s3://") {

			s3url, err := url.Parse(s3)

			if err != nil {
				errorf("Error parsing S3 URL %s: %s", s3, err)
			}

			bucket = s3url.Host
			path = strings.TrimPrefix(s3url.Path, "/")
		} else {
			slash := strings.IndexRune(*s3path, '/')

			if slash > 0 {
				bucket = s3[:slash]
				path = s3[slash+1:]
			}
		}

		backend, err = config.NewAWSConfig(bucket, path)

		if err != nil {
			errorf("Error initializing AWS config with S3 path %s/%s: %s", bucket, path, err)
		}

	} else if *localPath != "" {
		backend = &config.LocalConfig{Root: *localPath}
	} else {
		help(1)
	}

	vpn, err := vpn.BootVPN(backend, absRoot)

	if err != nil {
		errorf("Error starting VPN: %s", err)
	}

	defer vpn.Shutdown()

	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, syscall.SIGINT, syscall.SIGTERM)

	<-sigChannel
}

func errorf(format string, v ...interface{}) {
	format += "\n"
	fmt.Printf(format, v...)
	os.Exit(1)
}

func help(exit int) {
	getopt.PrintUsage(os.Stdout)
	os.Exit(exit)
}

func handleVerify() int {
	logger := log.New("tls-verify")
	logger.Debugf("Verifying %s", os.Getenv("peer_cert"))

	configPath := os.Getenv("config")
	vpnRoot := filepath.Dir(configPath)

	ok, err := ca.CheckCertificate(filepath.Join(vpnRoot, "capath"), os.Getenv("peer_cert"))

	if err != nil {
		logger.Errorf("Error checking certificate %s", err)
	}

	if !ok {
		return 1
	}

	return 0
}
