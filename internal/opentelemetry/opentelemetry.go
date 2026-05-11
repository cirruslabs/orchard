package opentelemetry

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	DefaultLogger = otellogglobal.Logger("")
	DefaultMeter  = otel.Meter("")
	DefaultTracer = otel.Tracer("")
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
	if err := setupTracerProvider(ctx); err != nil {
		return err
	}
	if err := setupLoggerProvider(ctx); err != nil {
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

func setupTracerProvider(ctx context.Context) error {
	httpExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return err
	}

	grpcExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(httpExporter),
		sdktrace.WithBatcher(grpcExporter),
	)

	otel.SetTracerProvider(tracerProvider)

	return nil
}

func setupLoggerProvider(ctx context.Context) error {
	httpExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return err
	}

	grpcExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return err
	}

	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(httpExporter)),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(grpcExporter)),
	)

	otellogglobal.SetLoggerProvider(loggerProvider)

	return nil
}
