package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"go.uber.org/zap"
	"net"
	"net/http"
)

type Controller struct {
	dataDir    string
	listenAddr string
	tlsConfig  *tls.Config
	listener   net.Listener
	httpServer *http.Server
	store      *storepkg.Store
	logger     *zap.SugaredLogger
}

func New(opts ...Option) (*Controller, error) {
	controller := &Controller{}

	// Apply options
	for _, opt := range opts {
		opt(controller)
	}

	// Apply defaults
	if controller.dataDir == "" {
		return nil, fmt.Errorf("%w: please specify the data directory path with WithDataDir()")
	}
	if controller.listenAddr == "" {
		controller.listenAddr = ":6120"
	}
	if controller.logger == nil {
		controller.logger = zap.NewNop().Sugar()
	}

	// Instantiate controller
	store, err := storepkg.New(controller.dbPath())
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
		Handler: controller.initAPI(),
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

	// Run the janitor so that inactive workers
	// will eventually be removed from the DB
	go func() {
		err := controller.runJanitor(controller.store)
		if err != nil {
			panic(err)
		}
	}()

	// A helper function to shut down the HTTP server on context cancellation
	go func() {
		<-ctx.Done()
		controller.httpServer.Shutdown(ctx)
	}()

	if err := controller.httpServer.Serve(controller.listener); err != nil {
		return err
	}

	return nil
}
