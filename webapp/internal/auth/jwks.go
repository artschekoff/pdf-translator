package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

type JWKS struct {
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	url     string
	ttl     time.Duration
	fetched time.Time
}

func NewJWKS(jwksURL string) *JWKS {
	return &JWKS{url: jwksURL, ttl: 5 * time.Minute, keys: make(map[string]*rsa.PublicKey)}
}

func (j *JWKS) GetKey(kid string) (*rsa.PublicKey, error) {
	j.mu.RLock()
	if time.Since(j.fetched) < j.ttl {
		k, ok := j.keys[kid]
		j.mu.RUnlock()
		if ok {
			return k, nil
		}
	} else {
		j.mu.RUnlock()
	}
	return j.refresh(kid)
}

func (j *JWKS) refresh(kid string) (*rsa.PublicKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	for _, k := range payload.Keys {
		nBytes, _ := base64.RawURLEncoding.DecodeString(k.N)
		eBytes, _ := base64.RawURLEncoding.DecodeString(k.E)
		e := int(new(big.Int).SetBytes(eBytes).Int64())
		pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
		j.keys[k.Kid] = pub
	}
	j.fetched = time.Now()

	if k, ok := j.keys[kid]; ok {
		return k, nil
	}
	for _, k := range j.keys {
		return k, nil
	}
	return nil, fmt.Errorf("jwks: key %q not found", kid)
}
