package kvstore

import (
	"crypto/tls"
	"fmt"
	"net"
	"path"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cpuguy83/drax"
	"github.com/cpuguy83/drax/api/client"
	"github.com/docker/docker/pkg/discovery"
	"github.com/docker/docker/pkg/tlsconfig"
	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
)

const (
	defaultDiscoveryPath               = "docker/nodes"
	name                 store.Backend = "docker"
)

func init() {
	libkv.AddStore(name, initLibKV)
	discovery.Register(string(name), &Discovery{})
}

type Discovery struct {
	c         *drax.Cluster
	heartbeat time.Duration
	ttl       time.Duration
	prefix    string
	path      string
	store     store.Store
}

func (s *Discovery) Initialize(addr string, heartbeat time.Duration, ttl time.Duration, clusterOpts map[string]string) error {
	var (
		parts           = strings.SplitN(addr, "/", 2)
		err             error
		serverTLSConfig *tls.Config
	)
	if len(parts) == 2 {
		s.prefix = parts[1]
	}

	s.heartbeat = heartbeat
	s.ttl = ttl

	// Use a custom path if specified in discovery options
	dpath := defaultDiscoveryPath
	if clusterOpts["kv.path"] != "" {
		dpath = clusterOpts["kv.path"]
	}

	s.path = path.Join(s.prefix, dpath)

	if clusterOpts["kv.cacertfile"] != "" && clusterOpts["kv.certfile"] != "" && clusterOpts["kv.keyfile"] != "" {
		log.Info("Initializing discovery with TLS")

		tlsOpts := tlsconfig.Options{
			CAFile:   clusterOpts["kv.cacertfile"],
			CertFile: clusterOpts["kv.certfile"],
			KeyFile:  clusterOpts["kv.keyfile"],
		}

		serverTLSConfig, err = tlsconfig.Server(tlsOpts)
		if err != nil {
			return err
		}
	} else {
		log.Info("Initializing discovery without TLS")
	}

	peer := clusterOpts["kv.peer"]
	// TODO: make the home path inherit from docker somehow?
	c, err := newCluster(addr, peer, "/var/lib/dockerkv", serverTLSConfig)
	if err != nil {
		return err
	}
	s.c = c
	s.store = c.KVStore()
	return nil
}

// Watch the store until either there's a store error or we receive a stop request.
// Returns false if we shouldn't attempt watching the store anymore (stop request received).
func (s *Discovery) watchOnce(stopCh <-chan struct{}, watchCh <-chan []*store.KVPair, discoveryCh chan discovery.Entries, errCh chan error) bool {
	for {
		select {
		case pairs := <-watchCh:
			if pairs == nil {
				return true
			}

			log.WithField("discovery", name).Debugf("Watch triggered with %d nodes", len(pairs))

			// Convert `KVPair` into `discovery.Entry`.
			addrs := make([]string, len(pairs))
			for _, pair := range pairs {
				addrs = append(addrs, string(pair.Value))
			}

			entries, err := discovery.CreateEntries(addrs)
			if err != nil {
				errCh <- err
			} else {
				discoveryCh <- entries
			}
		case <-stopCh:
			// We were requested to stop watching.
			return false
		}
	}
}

// Watch is exported
func (s *Discovery) Watch(stopCh <-chan struct{}) (<-chan discovery.Entries, <-chan error) {
	ch := make(chan discovery.Entries)
	errCh := make(chan error)

	go func() {
		defer close(ch)
		defer close(errCh)

		// Forever: Create a store watch, watch until we get an error and then try again.
		// Will only stop if we receive a stopCh request.
		for {
			// Create the path to watch if it does not exist yet
			exists, err := s.store.Exists(s.path)
			if err != nil {
				errCh <- err
			}
			if !exists {
				if err := s.store.Put(s.path, []byte(""), &store.WriteOptions{IsDir: true}); err != nil {
					errCh <- err
				}
			}

			// Set up a watch.
			watchCh, err := s.store.WatchTree(s.path, stopCh)
			if err != nil {
				errCh <- err
			} else {
				if !s.watchOnce(stopCh, watchCh, ch, errCh) {
					return
				}
			}

			// If we get here it means the store watch channel was closed. This
			// is unexpected so let's retry later.
			errCh <- fmt.Errorf("Unexpected watch error")
			time.Sleep(s.heartbeat)
		}
	}()
	return ch, errCh
}

// Register is exported
func (s *Discovery) Register(addr string) error {
	opts := &store.WriteOptions{TTL: s.ttl}
	return s.store.Put(path.Join(s.path, addr), []byte(addr), opts)
}

// Store returns the underlying store used by KV discovery.
func (s *Discovery) Store() store.Store {
	return s.store
}

// Prefix returns the store prefix
func (s *Discovery) Prefix() string {
	return s.prefix
}

// New creates a new kv cluster.
// If a peer is specified it will join to the peer
func newCluster(addr, peer, home string, tlsConfig *tls.Config) (c *drax.Cluster, err error) {
	if tlsConfig != nil {
		return newWithTLS(addr, peer, home, tlsConfig)
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	dialerFn := func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("tcp", addr, timeout)
	}
	return drax.New(l, dialerFn, home, addr, peer, &logWriter{})
}

func newWithTLS(addr, peer, home string, tlsConfig *tls.Config) (*drax.Cluster, error) {
	l, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return nil, err
	}

	dialerFn := func(addr string, timeout time.Duration) (net.Conn, error) {
		d := &net.Dialer{Timeout: timeout}
		return tls.DialWithDialer(d, "tcp", addr, tlsConfig)
	}
	return drax.New(l, dialerFn, home, addr, peer, &logWriter{})
}

func newClient(addr string, timeout time.Duration, tlsConfig *tls.Config) store.Store {
	if tlsConfig != nil {
		return newClientWithTLS(addr, timeout, tlsConfig)
	}

	dialerFn := func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("tcp", addr, timeout)
	}
	return client.New(addr, timeout, dialerFn)
}

func newClientWithTLS(addr string, timeout time.Duration, tlsConfig *tls.Config) store.Store {
	dialerFn := func(addr string, timeout time.Duration) (net.Conn, error) {
		d := &net.Dialer{Timeout: timeout}
		return tls.DialWithDialer(d, "tcp", addr, tlsConfig)
	}
	return client.New(addr, timeout, dialerFn)
}

func initLibKV(addrs []string, options *store.Config) (store.Store, error) {
	return newClient(addrs[0], options.ConnectionTimeout, options.TLS), nil
}
