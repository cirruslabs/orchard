package controller

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/controller/store/badger"
	"go.uber.org/zap"
	"net"
	"net/http"
	"time"
)

const (
	DefaultPort       = 6120
	DefaultServerName = "orchard-controller"
)

var ErrInitFailed = errors.New("controller initialization failed")

type Controller struct {
	dataDir    *DataDir
	listenAddr string
	tlsConfig  *tls.Config
	listener   net.Listener
	httpServer *http.Server
	store      storepkg.Store
	logger     *zap.SugaredLogger
}

func New(opts ...Option) (*Controller, error) {
	controller := &Controller{}

	// Apply options
	for _, opt := range opts {
		opt(controller)
	}

	// Apply defaults
	if controller.dataDir == nil {
		return nil, fmt.Errorf("%w: please specify the data directory path with WithDataDir()",
			ErrInitFailed)
	}
	if controller.listenAddr == "" {
		controller.listenAddr = fmt.Sprintf(":%d", DefaultPort)
	}
	if controller.logger == nil {
		controller.logger = zap.NewNop().Sugar()
	}

	// Instantiate controller
	store, err := badger.NewBadgerStore(controller.dataDir.DBPath())
	if err != nil {
		return nil, err
	}
	controller.store = store

	listener, err := net.Listen("tcp", controller.listenAddr)
	if err != nil {
		return nil, err
	}
	if controller.tlsConfig != nil {
		controller.listener = tls.NewListener(listener, controller.tlsConfig)
	} else {
		controller.listener = listener
	}

	controller.httpServer = &http.Server{
		Handler:     controller.initAPI(),
		ReadTimeout: 5 * time.Second,
	}

	return controller, nil
}

func (controller *Controller) Run(ctx context.Context) error {
	// Run the scheduler so that each VM will eventually
	// be assigned to a specific Worker
	go func() {
		err := runScheduler(controller.store)
		if err != nil {
			panic(err)
		}
	}()

	// A helper function to shut down the HTTP server on context cancellation
	go func() {
		<-ctx.Done()

		if err := controller.httpServer.Shutdown(ctx); err != nil {
			controller.logger.Errorf("failed to cleanly shutdown the HTTP server: %v", err)
		}
	}()

	if err := controller.httpServer.Serve(controller.listener); err != nil {
		return err
	}

	return nil
}
