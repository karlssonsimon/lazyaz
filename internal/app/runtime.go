package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"azure-storage/internal/azure/blob"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/azure/servicebus"
	blobrpc "azure-storage/internal/blobapp/rpc"
	"azure-storage/internal/cache"
	kvrpc "azure-storage/internal/kvapp/rpc"
	sbrpc "azure-storage/internal/sbapp/rpc"
)

type Runtime struct {
	dir string

	blobSocket string
	kvSocket   string
	sbSocket   string

	blobDB *cache.DB
	kvDB   *cache.DB
	sbDB   *cache.DB

	blobServer *blobrpc.Server
	kvServer   *kvrpc.Server
	sbServer   *sbrpc.Server

	closeOnce sync.Once
}

func NewRuntime(blobSvc *blob.Service, sbSvc *servicebus.Service, kvSvc *keyvault.Service) (*Runtime, error) {
	dir, err := os.MkdirTemp("", "aztui-rpc-*")
	if err != nil {
		return nil, fmt.Errorf("create runtime directory: %w", err)
	}
	rt := &Runtime{
		dir:        dir,
		blobSocket: filepath.Join(dir, "blob.sock"),
		kvSocket:   filepath.Join(dir, "kv.sock"),
		sbSocket:   filepath.Join(dir, "sb.sock"),
	}
	if err := rt.start(blobSvc, sbSvc, kvSvc); err != nil {
		rt.Close()
		return nil, err
	}
	return rt, nil
}

func (r *Runtime) start(blobSvc *blob.Service, sbSvc *servicebus.Service, kvSvc *keyvault.Service) error {
	var err error
	r.blobDB = openServerDB("blob")
	r.kvDB = openServerDB("kv")
	r.sbDB = openServerDB("sb")

	r.blobServer, err = blobrpc.NewServer(r.blobSocket, blobSvc, r.blobDB)
	if err != nil {
		return fmt.Errorf("start blob server: %w", err)
	}
	r.kvServer, err = kvrpc.NewServer(r.kvSocket, kvSvc, r.kvDB)
	if err != nil {
		return fmt.Errorf("start key vault server: %w", err)
	}
	r.sbServer, err = sbrpc.NewServer(r.sbSocket, sbSvc, r.sbDB)
	if err != nil {
		return fmt.Errorf("start service bus server: %w", err)
	}
	go r.blobServer.Serve()
	go r.kvServer.Serve()
	go r.sbServer.Serve()
	return nil
}

func (r *Runtime) NewBlobClient() (*blobrpc.Client, error) { return blobrpc.Dial(r.blobSocket) }
func (r *Runtime) NewKVClient() (*kvrpc.Client, error)     { return kvrpc.Dial(r.kvSocket) }
func (r *Runtime) NewSBClient() (*sbrpc.Client, error)     { return sbrpc.Dial(r.sbSocket) }

func (r *Runtime) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		if r.blobServer != nil && closeErr == nil {
			closeErr = r.blobServer.Close()
		}
		if r.kvServer != nil && closeErr == nil {
			closeErr = r.kvServer.Close()
		}
		if r.sbServer != nil && closeErr == nil {
			closeErr = r.sbServer.Close()
		}
		if r.blobDB != nil && closeErr == nil {
			closeErr = r.blobDB.Close()
		}
		if r.kvDB != nil && closeErr == nil {
			closeErr = r.kvDB.Close()
		}
		if r.sbDB != nil && closeErr == nil {
			closeErr = r.sbDB.Close()
		}
		if r.dir != "" {
			_ = os.RemoveAll(r.dir)
		}
	})
	return closeErr
}

func openServerDB(name string) *cache.DB {
	path, err := cache.DefaultServerDBPath(name)
	if err != nil {
		return nil
	}
	db, err := cache.OpenDB(path)
	if err != nil {
		return nil
	}
	return db
}
