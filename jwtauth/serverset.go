package jwtauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pentops/log.go/log"
	"github.com/pquerna/cachecontrol/cacheobject"
	"golang.org/x/sync/errgroup"
	"gopkg.in/square/go-jose.v2"
)

type KeySource interface {
	Refresh(ctx context.Context) (time.Duration, error)
	Keys() []jose.JSONWebKey
}

// JWKSManager merges multiple JWKS sources
type JWKSManager struct {
	servers     []KeySource
	jwksBytes   []byte
	mutex       sync.RWMutex
	jwksMutex   sync.RWMutex
	initialLoad chan error
}

func NewKeyManager(sources ...KeySource) *JWKSManager {
	ss := &JWKSManager{
		servers:   sources,
		jwksBytes: []byte(`{"keys":[]}`),
	}
	return ss
}

func NewKeyManagerFromURLs(urls ...string) (*JWKSManager, error) {
	servers := make([]KeySource, len(urls))

	client := &http.Client{
		Timeout: time.Second * 5,
	}
	for idx, url := range urls {
		server := &HTTPKeySource{
			client: client,
			url:    url,
		}
		servers[idx] = server
	}

	ss := &JWKSManager{
		servers:     servers,
		jwksBytes:   []byte(`{"keys":[]}`),
		initialLoad: make(chan error),
	}

	return ss, nil
}

func (km *JWKSManager) WaitForKeys(ctx context.Context) error {
	return <-km.initialLoad

}

func (km *JWKSManager) logError(ctx context.Context, err error) {
	log.WithError(ctx, err).Error("Failed to load JWKS")
}

// Run fetches once from each source, then refreshes the keys based on cache
// control headers.
func (km *JWKSManager) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, server := range km.servers {
		server := server
		duration, err := server.Refresh(ctx)
		if err != nil {
			// Initial Fetch must work
			go func() {
				km.initialLoad <- err
				close(km.initialLoad)
			}()
			return err
		}
		km.mergeKeys()

		eg.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(duration):
					duration, err = server.Refresh(ctx)
					if err != nil {
						km.logError(ctx, err)
					} else {
						km.mergeKeys()
					}
				}
			}
		})
	}

	close(km.initialLoad)

	return eg.Wait()
}

func (km *JWKSManager) mergeKeys() {
	km.jwksMutex.Lock()
	defer km.jwksMutex.Unlock()
	keys := make([]jose.JSONWebKey, 0, 1)

	for _, server := range km.servers {
		serverKeys := server.Keys()
		keys = append(keys, serverKeys...)
	}

	keySet := jose.JSONWebKeySet{
		Keys: keys,
	}

	keyBytes, err := json.Marshal(keySet)
	if err != nil {
		return
	}
	km.jwksBytes = keyBytes
}

func (ss *JWKSManager) JWKS() []byte {
	ss.jwksMutex.RLock()
	defer ss.jwksMutex.RUnlock()
	return ss.jwksBytes
}

func (ss *JWKSManager) GetKeys(keyID string) ([]jose.JSONWebKey, error) {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()
	keys := make([]jose.JSONWebKey, 0, 1)

	for _, server := range ss.servers {
		serverKeys := server.Keys()
		for _, key := range serverKeys {
			if key.KeyID == keyID {
				keys = append(keys, key)
			}
		}
	}

	return keys, nil
}

type HTTPKeySource struct {
	keyset *jose.JSONWebKeySet
	url    string
	client *http.Client
	lock   sync.RWMutex
}

func (ss *HTTPKeySource) Keys() []jose.JSONWebKey {
	ss.lock.RLock()
	defer ss.lock.RUnlock()
	return ss.keyset.Keys
}

func (ss *HTTPKeySource) Refresh(ctx context.Context) (time.Duration, error) {
	req, err := http.NewRequest("GET", ss.url, nil)
	if err != nil {
		return 0, err
	}

	res, err := ss.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", ss.url, err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET %s: %s", ss.url, res.Status)
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", ss.url, err)
	}

	keyset := &jose.JSONWebKeySet{}

	if err := json.Unmarshal(bodyBytes, keyset); err != nil {
		return 0, fmt.Errorf("Parsing %s: %w", ss.url, err)
	}

	refreshTime := parseCacheControlHeader(res.Header.Get("Cache-Control"))

	ss.lock.Lock()
	defer ss.lock.Unlock()
	ss.keyset = keyset
	return refreshTime, nil
}

func parseCacheControlHeader(raw string) time.Duration {
	respDir, err := cacheobject.ParseResponseCacheControl(raw)
	if err != nil || respDir.NoCachePresent || respDir.NoStore || respDir.PrivatePresent {
		return time.Second * 30
	}
	if respDir.MaxAge > 0 {
		return time.Duration(respDir.MaxAge) * time.Second
	}

	return time.Second * 30
}
