package opentelemetry

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
)

var (
	DefaultMeter = otel.Meter("")
)

func Configure(ctx context.Context) error {
	if err := setupMeterProvider(ctx); err != nil {
		return err
	}

	return nil
}

func setupMeterProvider(ctx context.Context) error {
	meterExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(meterExporter)),
	)

	otel.SetMeterProvider(meterProvider)

	return nil
}
