package controller

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	"github.com/cirruslabs/orchard/internal/controller/proxy"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/controller/store/badger"
	"github.com/cirruslabs/orchard/internal/netconstants"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInitFailed      = errors.New("controller initialization failed")
	ErrAdminTaskFailed = errors.New("controller administrative task failed")
)

type Controller struct {
	dataDir              *DataDir
	listenAddr           string
	tlsConfig            *tls.Config
	listener             net.Listener
	httpServer           *http.Server
	insecureAuthDisabled bool
	store                storepkg.Store
	logger               *zap.SugaredLogger
	grpcServer           *grpc.Server
	workerNotifier       *notifier.Notifier
	proxy                *proxy.Proxy

	rpc.UnimplementedControllerServer
}

func New(opts ...Option) (*Controller, error) {
	controller := &Controller{
		workerNotifier: notifier.NewNotifier(),
		proxy:          proxy.NewProxy(),
	}

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
		controller.listenAddr = fmt.Sprintf(":%d", netconstants.DefaultControllerPort)
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

	apiServer := controller.initAPI()

	controller.grpcServer = grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: 30 * time.Second,
		}),
	)
	rpc.RegisterControllerServer(controller.grpcServer, controller)

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Content-Type") == "application/grpc" {
			controller.grpcServer.ServeHTTP(writer, request)
		} else {
			apiServer.ServeHTTP(writer, request)
		}
	})

	controller.httpServer = &http.Server{
		Handler:           h2c.NewHandler(handler, &http2.Server{}),
		ReadHeaderTimeout: 60 * time.Second,
	}

	return controller, nil
}

func (controller *Controller) EnsureServiceAccount(serviceAccount *v1.ServiceAccount) error {
	if serviceAccount.Name == "" {
		return fmt.Errorf("%w: attempted to create a service account with an empty name",
			ErrAdminTaskFailed)
	}

	if serviceAccount.Token == "" {
		return fmt.Errorf("%w: attempted to create a service account with an empty token",
			ErrAdminTaskFailed)
	}

	serviceAccount.CreatedAt = time.Now()

	return controller.store.Update(func(txn storepkg.Transaction) error {
		return txn.SetServiceAccount(serviceAccount)
	})
}

func (controller *Controller) DeleteServiceAccount(name string) error {
	return controller.store.Update(func(txn storepkg.Transaction) error {
		return txn.DeleteServiceAccount(name)
	})
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

func (controller *Controller) Address() string {
	hostPort := strings.ReplaceAll(controller.listener.Addr().String(), "[::]", "127.0.0.1")

	if controller.tlsConfig != nil {
		return fmt.Sprintf("https://%s", hostPort)
	}

	return fmt.Sprintf("http://%s", hostPort)
}
