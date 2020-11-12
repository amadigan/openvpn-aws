package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sync"

	"github.com/amadigan/openvpn-aws/internal/config"
	"github.com/amadigan/openvpn-aws/internal/log"
)

var logger = log.New("ui-server")

type UIServer struct {
	caCertificate []byte
	listener      net.Listener
	clientConfig  []byte
	vpnConfig     []byte
	vpnConfigTag  string
	cache         map[string]*cacheItem
	lock          sync.RWMutex
}

type cacheItem struct {
	content []byte
	headers map[string]string
	tag     string
}

func (s *UIServer) UpdateConfig(configFile *config.ConfigFile, backend config.ConfigurationBackend) error {
	keyStrength := configFile.KeyStrength

	if keyStrength < 2048 {
		keyStrength = 4096
	}

	config := struct {
		Bits    int  `json:"bits"`
		Dynamic bool `json:"dynamic"`
	}{keyStrength, true}

	clientConfig, err := json.Marshal(config)

	if err != nil {
		return err
	}

	s.lock.Lock()
	s.clientConfig = clientConfig
	tag := s.vpnConfigTag
	s.lock.Unlock()

	base, newTag, _ := s.getCacheItem("config.ovpn", tag)

	if base != nil {
		buf := bytes.NewBuffer(make([]byte, 0, 4096))
		buf.Write(base)
		buf.WriteString(fmt.Sprintf("\nremote %s 1194\n", configFile.DomainName))
		buf.WriteString("\n<ca>\n")
		buf.Write(s.caCertificate)
		buf.WriteString("\n</ca>\n")

		s.lock.Lock()
		s.clientConfig = buf.Bytes()
		s.vpnConfigTag = newTag
		s.lock.Unlock()
	}

	for name, file := range UIBundle {
		if file.Headers["content-type"] == MarkdownType {
			err = s.updateCache(name, backend)

			if err != nil {
				logger.Warnf("Failed to update cache for %s, %s", name, err)
			}
		}
	}

	return nil
}

func (s *UIServer) updateCache(name string, backend config.ConfigurationBackend) error {
	s.lock.RLock()
	item := s.cache[name]
	s.lock.RUnlock()

	var tag string

	if item != nil {
		tag = item.tag
	}

	reader, newTag, err := backend.FetchFile(name, tag)

	if err != nil {
		return err
	}

	if reader == nil && newTag == "" {
		// 404
		if item != nil {
			s.lock.Lock()
			delete(s.cache, name)
			s.lock.Unlock()
		}

		return nil
	}

	if reader != nil {
		bs, err := ioutil.ReadAll(reader)
		reader.Close()

		if err != nil {
			return err
		}

		newItem := cacheItem{content: bs, tag: newTag}
		s.lock.Lock()
		s.cache[name] = &newItem
		s.lock.Unlock()
	}

	return nil
}

func (s *UIServer) getCacheItem(name string, ifNotTag string) ([]byte, string, map[string]string) {
	s.lock.RLock()
	cacheItem := s.cache[name]
	s.lock.RUnlock()

	if cacheItem != nil {
		if cacheItem.tag == ifNotTag {
			return nil, cacheItem.tag, cacheItem.headers
		}
		return cacheItem.content, cacheItem.tag, cacheItem.headers
	}

	file := UIBundle[name]

	return file.Content, file.Headers["etag"], file.Headers
}
