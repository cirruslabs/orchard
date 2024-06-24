package opentelemetry

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"os"
)

var (
	DefaultMeter = otel.Meter("")
)

func Configure(ctx context.Context) error {
	// Avoid logging errors when local OpenTelemetry Collector is not available, for example:
	// "failed to upload metrics: [...]: dial tcp 127.0.0.1:4318: connect: connection refused"
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		// do nothing
	}))

	// Work around https://github.com/open-telemetry/opentelemetry-go/issues/4834
	if _, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT"); !ok {
		if err := os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"); err != nil {
			return err
		}
	}

	if err := setupMeterProvider(ctx); err != nil {
		return err
	}

	return nil
}

func setupMeterProvider(ctx context.Context) error {
	httpExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return err
	}

	grpcExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(httpExporter)),
		metric.WithReader(metric.NewPeriodicReader(grpcExporter)),
	)

	otel.SetMeterProvider(meterProvider)

	return nil
}
