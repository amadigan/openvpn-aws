package vpn

import (
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/ca"
	"github.com/amadigan/openvpn-aws/internal/config"
	"path/filepath"
	"sync"
	"time"
)

type userManager struct {
	backend            config.ConfigurationBackend
	certificateManager *ca.CertificateManager
	confFile           *config.ConfigFile
	netinfo            *config.NetworkInfo
	users              map[string]*userKeys
	userGroups         map[string][]string
	lock               sync.RWMutex
}

type userKeys struct {
	keyById   map[string]string
	keyByHash map[string]string
}

type vpnUser struct {
	keys   map[string]bool
	config *config.UserConfig
}

func initUserManager(backend config.ConfigurationBackend, root string) (*userManager, error) {
	certManager, err := ca.CreateCertificateManager(filepath.Join(root, "capath"))

	if err != nil {
		return nil, err
	}

	return &userManager{
		backend:            backend,
		certificateManager: certManager,
		users:              make(map[string]*userKeys),
	}, nil
}

func (c *userManager) update() (map[string]*vpnUser, *time.Duration, error) {
	file, err := c.backend.FetchFile("vpn.conf")

	if file == nil {
		return nil, nil, err
	}

	confFile, err := config.ParseConfig(file)
	file.Close()
	if err != nil {
		return nil, nil, err
	}

	userConfs, err := c.buildUserConfigs(confFile)

	return userConfs, confFile.WatchTime, err
}

func (c *userManager) buildUserConfigs(confFile *config.ConfigFile) (map[string]*vpnUser, error) {
	netinfo, err := c.backend.FetchNetworkInfo()

	if netinfo == nil {
		logger.Errorf("Failed to fetch network info: %s", err)
		return nil, err
	}

	c.lock.Lock()
	c.confFile = confFile
	c.netinfo = netinfo
	c.lock.Unlock()

	userGroups := make(map[string][]string)

	for groupName := range confFile.Groups {
		users, err := c.backend.FetchGroup(groupName)

		if err != nil {
			return nil, err
		}

		for _, user := range users {
			userGroups[user] = append(userGroups[user], groupName)
		}
	}

	for userName := range confFile.Users {
		_, exists := userGroups[userName]

		if !exists {
			userGroups[userName] = nil
		}
	}

	configs, err := c.updateKeys(userGroups)

	if err != nil {
		return nil, err
	}

	for user, info := range configs {
		userConf, err := confFile.GetUserConfig(user, userGroups[user], netinfo)

		if err != nil {
			return nil, err
		}

		info.config = userConf
	}

	c.lock.Lock()
	c.userGroups = userGroups
	c.lock.Unlock()

	return configs, nil
}

func (c *userManager) updateKeys(userGroups map[string][]string) (map[string]*vpnUser, error) {
	configs := make(map[string]*vpnUser, len(userGroups))

	for user, _ := range userGroups {
		keys, err := c.backend.FetchKeys(user)

		if err != nil {
			return nil, err
		}

		if len(keys) != 0 {
			keyIds := make(map[string]bool)

			for _, keyId := range keys {
				keyIds[keyId] = true
			}

			newKeys := make(map[string]string, len(keyIds))

			c.lock.RLock()

			userEntry := c.users[user]

			update := userEntry == nil || len(keyIds) != len(userEntry.keyById)

			for _, keyId := range keys {
				if userEntry == nil {
					newKeys[keyId] = ""
				} else if _, exists := userEntry.keyById[keyId]; !exists {
					newKeys[keyId] = ""
					update = true
				}
			}

			c.lock.RUnlock()

			for keyId, _ := range newKeys {
				key, err := c.backend.FetchKey(user, keyId)

				if err != nil {
					return nil, err
				}

				if key != nil {
					publicKey, err := ca.ParsePublicKey(key)

					if err != nil {
						return nil, err
					}

					hash, err := c.certificateManager.Add(user, keyId, publicKey)

					if err != nil {
						return nil, err
					}

					newKeys[keyId] = hash
				}
			}

			if update {
				removedKeys := make([]string, 0, len(keyIds))

				c.lock.Lock()

				userEntry = c.users[user]

				if userEntry == nil {
					userEntry = &userKeys{
						keyById:   make(map[string]string),
						keyByHash: make(map[string]string),
					}

					c.users[user] = userEntry
				}

				for keyId, keyHash := range userEntry.keyById {
					if _, exists := keyIds[keyId]; !exists {
						delete(userEntry.keyById, keyId)
						delete(userEntry.keyByHash, keyHash)
						removedKeys = append(removedKeys, keyId)
					}
				}

				for keyId, keyHash := range newKeys {
					userEntry.keyById[keyId] = keyHash
					userEntry.keyByHash[keyHash] = keyId
				}

				c.lock.Unlock()

				for _, keyId := range removedKeys {
					c.certificateManager.Remove(keyId)
				}
			}

			configs[user] = &vpnUser{keys: keyIds}
		}
	}

	return configs, nil
}

func (c *userManager) authenticateUser(user, keyHash string) (config *config.UserConfig, keyId string, err error) {
	c.lock.RLock()
	userInfo := c.users[user]

	if userInfo != nil {
		keyId = userInfo.keyByHash[keyHash]
	}

	c.lock.RUnlock()

	if keyId == "" {
		return nil, keyId, fmt.Errorf("User %s not found", user)
	}

	userKeys, err := c.backend.FetchKeys(user)

	if userKeys == nil && err == nil {
		return nil, keyId, fmt.Errorf("User %s does not exist", user)
	}

	keyExists := false

	for _, key := range userKeys {
		if key == keyId {
			keyExists = true
			break
		}
	}

	if !keyExists {
		return nil, keyId, fmt.Errorf("Key %s does not exist for user %s", keyId, user)
	}

	groups, err := c.backend.FetchGroupsForUser(user)

	if err != nil {
		logger.Warnf("Error retrieving groups for user: %s", err)
	}

	if err != nil {
		c.lock.RLock()
		defer c.lock.RUnlock()
		groups = c.userGroups[user]
	} else {
		c.lock.Lock()
		c.userGroups[user] = groups
		c.lock.Unlock()
		c.lock.RLock()
		defer c.lock.RUnlock()
	}

	logger.Debugf("Groups for user %s: %v", user, groups)
	config, err = c.confFile.GetUserConfig(user, groups, c.netinfo)
	return config, keyId, err
}
