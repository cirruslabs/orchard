//nolint:testpackage // telemetry tests exercise controller internals directly
package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type execTelemetryHarness struct {
	telemetry      *execTelemetry
	logExporter    *memoryLogExporter
	metricReader   *sdkmetric.ManualReader
	tracerProvider *sdktrace.TracerProvider
	spanRecorder   *tracetest.SpanRecorder
}

func newExecTelemetryHarness(t *testing.T) *execTelemetryHarness {
	t.Helper()

	logExporter := &memoryLogExporter{}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExporter)),
	)
	t.Cleanup(func() {
		require.NoError(t, loggerProvider.Shutdown(context.Background()))
	})

	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	t.Cleanup(func() {
		require.NoError(t, meterProvider.Shutdown(context.Background()))
	})

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		require.NoError(t, tracerProvider.Shutdown(context.Background()))
	})

	telemetry, err := newExecTelemetry(
		meterProvider.Meter("exec-telemetry-test"),
		tracerProvider.Tracer("exec-telemetry-test"),
		loggerProvider.Logger("exec-telemetry-test"),
	)
	require.NoError(t, err)

	return &execTelemetryHarness{
		telemetry:      telemetry,
		logExporter:    logExporter,
		metricReader:   metricReader,
		tracerProvider: tracerProvider,
		spanRecorder:   spanRecorder,
	}
}

type memoryLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (exporter *memoryLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()

	for _, record := range records {
		exporter.records = append(exporter.records, record.Clone())
	}

	return nil
}

func (exporter *memoryLogExporter) Shutdown(context.Context) error {
	return nil
}

func (exporter *memoryLogExporter) ForceFlush(context.Context) error {
	return nil
}

func (exporter *memoryLogExporter) snapshot() []sdklog.Record {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()

	records := make([]sdklog.Record, 0, len(exporter.records))
	for _, record := range exporter.records {
		records = append(records, record.Clone())
	}

	return records
}

func TestExecTelemetryCapturesReconnectableSessionAcrossSignals(t *testing.T) {
	harness := newExecTelemetryHarness(t)
	spec := execSessionSpec{
		command:     "printf secret",
		interactive: true,
		tty:         true,
	}
	key := execSessionKey{vmName: "vm-1", sessionID: "session-1"}
	vm := &v1.VM{
		Meta:   v1.Meta{Name: "vm-1"},
		UID:    "uid-1",
		Worker: "worker-1",
	}

	setupTelemetry := harness.telemetry.startSetup(context.Background(), key, spec)
	setupTelemetry.setVM(vm)
	sessionTraceContext, sessionTelemetry := setupTelemetry.startSession()
	setupTelemetry.finish(nil)

	runRelease := make(chan struct{})
	sessionContext, sessionContextCancel := context.WithCancel(sessionTraceContext)
	session := newExecSessionWithContextAndSpec(
		sessionContext,
		sessionContextCancel,
		key,
		spec,
		spec.command,
		&fakeExec{
			run: func(context.Context, string, chan<- *execstream.Frame) error {
				<-runRelease

				return nil
			},
		},
		nil,
		nil,
		time.Minute,
		reconnectableExecSessionPolicy,
		sessionTelemetry,
	)

	firstSubscriber, err := session.attach(context.Background())
	require.NoError(t, err)
	session.start()
	session.detach(firstSubscriber)

	secondSubscriber, err := session.attach(context.Background())
	require.NoError(t, err)

	close(runRelease)
	<-session.done
	session.detach(secondSubscriber)

	require.NoError(t, harness.tracerProvider.ForceFlush(context.Background()))

	spans := harness.spanRecorder.Ended()
	setupSpan := findSpanByName(t, spans, "orchard.exec.setup")
	sessionSpan := findSpanByName(t, spans, "orchard.exec.session")
	attachmentSpans := findSpansByName(spans, "orchard.exec.attachment")
	require.Len(t, attachmentSpans, 2)
	require.Equal(t, setupSpan.SpanContext().SpanID(), sessionSpan.Parent().SpanID())
	for _, attachmentSpan := range attachmentSpans {
		require.Equal(t, sessionSpan.SpanContext().SpanID(), attachmentSpan.Parent().SpanID())
	}

	logs := harness.logExporter.snapshot()
	require.Equal(t, []string{
		execTelemetryEventAttached,
		execTelemetryEventSessionStarted,
		execTelemetryEventDetached,
		execTelemetryEventAttached,
		execTelemetryEventFinished,
		execTelemetryEventDetached,
	}, logBodies(logs))
	for _, record := range logs {
		require.Equal(t, sessionSpan.SpanContext().TraceID(), record.TraceID())
		require.NotContains(t, record.Body().AsString(), spec.command)
		require.NotContains(t, logAttributes(record), "command")
		require.Equal(t, execCommandHash(spec.command), logAttributes(record)["command_sha256"])
	}

	metrics := collectMetrics(t, harness.metricReader)
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.setup_attempts", nil))
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.sessions_started", nil))
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.sessions_finished",
		map[string]string{"outcome": execTelemetryOutcomeSuccess}))
	require.EqualValues(t, 0, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.active_sessions", nil))
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.attachments",
		map[string]string{"attachment_kind": execTelemetryAttachmentInitial}))
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.attachments",
		map[string]string{"attachment_kind": execTelemetryAttachmentReconnect}))
	require.EqualValues(t, 1, metricHistogramCount(t, metrics,
		"org.cirruslabs.orchard.controller.exec.setup_time"))
	require.EqualValues(t, 1, metricHistogramCount(t, metrics,
		"org.cirruslabs.orchard.controller.exec.session_time"))
}

func TestExecTelemetryRecordsSetupFailure(t *testing.T) {
	harness := newExecTelemetryHarness(t)
	setupTelemetry := harness.telemetry.startSetup(context.Background(),
		execSessionKey{vmName: "vm-1"},
		execSessionSpec{command: "echo hello"})

	setupErr := errors.New("setup failed")
	setupTelemetry.finish(setupErr)
	require.NoError(t, harness.tracerProvider.ForceFlush(context.Background()))

	setupSpan := findSpanByName(t, harness.spanRecorder.Ended(), "orchard.exec.setup")
	require.Equal(t, codes.Error, setupSpan.Status().Code)
	require.Equal(t, setupErr.Error(), setupSpan.Status().Description)

	logs := harness.logExporter.snapshot()
	require.Equal(t, []string{execTelemetryEventFailed}, logBodies(logs))

	metrics := collectMetrics(t, harness.metricReader)
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.setup_attempts",
		map[string]string{"outcome": execTelemetryOutcomeError}))
}

func TestExecTelemetryRecordsLegacyDetachAsCanceledSession(t *testing.T) {
	harness := newExecTelemetryHarness(t)
	spec := execSessionSpec{command: "sleep forever"}
	key := execSessionKey{vmName: "vm-1"}

	setupTelemetry := harness.telemetry.startSetup(context.Background(), key, spec)
	setupTelemetry.setVM(&v1.VM{Meta: v1.Meta{Name: "vm-1"}, UID: "uid-1", Worker: "worker-1"})
	sessionTraceContext, sessionTelemetry := setupTelemetry.startSession()
	setupTelemetry.finish(nil)

	sessionContext, sessionContextCancel := context.WithCancel(sessionTraceContext)
	session := newExecSessionWithContextAndSpec(
		sessionContext,
		sessionContextCancel,
		key,
		spec,
		spec.command,
		&fakeExec{
			run: func(ctx context.Context, _ string, _ chan<- *execstream.Frame) error {
				<-ctx.Done()

				return ctx.Err()
			},
		},
		nil,
		nil,
		time.Minute,
		legacyExecSessionPolicy,
		sessionTelemetry,
	)

	subscriber, err := session.attach(context.Background())
	require.NoError(t, err)
	session.start()
	session.detach(subscriber)
	<-session.done

	metrics := collectMetrics(t, harness.metricReader)
	require.EqualValues(t, 1, metricInt64Value(t, metrics,
		"org.cirruslabs.orchard.controller.exec.sessions_finished",
		map[string]string{"outcome": execTelemetryOutcomeCanceled}))
}

func TestExecTelemetryRecordsSessionFailure(t *testing.T) {
	harness := newExecTelemetryHarness(t)
	spec := execSessionSpec{command: "exit 1"}
	key := execSessionKey{vmName: "vm-1"}

	setupTelemetry := harness.telemetry.startSetup(context.Background(), key, spec)
	setupTelemetry.setVM(&v1.VM{Meta: v1.Meta{Name: "vm-1"}, UID: "uid-1", Worker: "worker-1"})
	sessionTraceContext, sessionTelemetry := setupTelemetry.startSession()
	setupTelemetry.finish(nil)

	sessionContext, sessionContextCancel := context.WithCancel(sessionTraceContext)
	session := newExecSessionWithContextAndSpec(
		sessionContext,
		sessionContextCancel,
		key,
		spec,
		spec.command,
		&fakeExec{
			run: func(context.Context, string, chan<- *execstream.Frame) error {
				return errors.New("command failed")
			},
		},
		nil,
		nil,
		time.Minute,
		legacyExecSessionPolicy,
		sessionTelemetry,
	)

	_, err := session.attach(context.Background())
	require.NoError(t, err)
	session.start()
	<-session.done
	require.NoError(t, harness.tracerProvider.ForceFlush(context.Background()))

	require.Contains(t, logBodies(harness.logExporter.snapshot()), execTelemetryEventFailed)
	sessionSpan := findSpanByName(t, harness.spanRecorder.Ended(), "orchard.exec.session")
	require.Equal(t, codes.Error, sessionSpan.Status().Code)
}

func TestExecCommandHashIsStableAndDoesNotExposeRawCommand(t *testing.T) {
	const command = "printf secret"

	require.Equal(t, execCommandHash(command), execCommandHash(command))
	require.NotEqual(t, command, execCommandHash(command))
	require.NotEqual(t, execCommandHash(command), execCommandHash(command+"!"))
}

func TestExecTelemetryMetricsAvoidHighCardinalityAttributes(t *testing.T) {
	harness := newExecTelemetryHarness(t)
	setupTelemetry := harness.telemetry.startSetup(context.Background(),
		execSessionKey{vmName: "vm-1", sessionID: "session-1"},
		execSessionSpec{command: "printf secret", interactive: true, tty: true})
	setupTelemetry.setVM(&v1.VM{Meta: v1.Meta{Name: "vm-1"}, UID: "uid-1", Worker: "worker-1"})
	setupTelemetry.finish(nil)

	metrics := collectMetrics(t, harness.metricReader)
	for _, metric := range flattenMetrics(metrics) {
		for _, attributes := range metricAttributeSets(metric) {
			require.NotContains(t, attributes, "vm_name")
			require.NotContains(t, attributes, "vm_uid")
			require.NotContains(t, attributes, "session_id")
			require.NotContains(t, attributes, "command_sha256")
		}
	}
}

func findSpanByName(
	t *testing.T,
	spans []sdktrace.ReadOnlySpan,
	name string,
) sdktrace.ReadOnlySpan {
	t.Helper()

	matches := findSpansByName(spans, name)
	require.Len(t, matches, 1)

	return matches[0]
}

func findSpansByName(spans []sdktrace.ReadOnlySpan, name string) []sdktrace.ReadOnlySpan {
	var matches []sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == name {
			matches = append(matches, span)
		}
	}

	return matches
}

func logBodies(records []sdklog.Record) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Body().AsString())
	}

	return result
}

func logAttributes(record sdklog.Record) map[string]string {
	result := map[string]string{}
	record.WalkAttributes(func(attribute otellog.KeyValue) bool {
		result[attribute.Key] = attribute.Value.AsString()

		return true
	})

	return result
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var metrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &metrics))

	return metrics
}

func flattenMetrics(metrics metricdata.ResourceMetrics) []metricdata.Metrics {
	var result []metricdata.Metrics
	for _, scopeMetrics := range metrics.ScopeMetrics {
		result = append(result, scopeMetrics.Metrics...)
	}

	return result
}

func metricInt64Value(
	t *testing.T,
	metrics metricdata.ResourceMetrics,
	name string,
	requiredAttributes map[string]string,
) int64 {
	t.Helper()

	for _, metric := range flattenMetrics(metrics) {
		if metric.Name != name {
			continue
		}

		sum, ok := metric.Data.(metricdata.Sum[int64])
		require.True(t, ok)

		for _, dataPoint := range sum.DataPoints {
			if attributesContain(dataPoint.Attributes.ToSlice(), requiredAttributes) {
				return dataPoint.Value
			}
		}
	}

	t.Fatalf("metric %q with attributes %v not found", name, requiredAttributes)

	return 0
}

func metricHistogramCount(t *testing.T, metrics metricdata.ResourceMetrics, name string) uint64 {
	t.Helper()

	for _, metric := range flattenMetrics(metrics) {
		if metric.Name != name {
			continue
		}

		histogram, ok := metric.Data.(metricdata.Histogram[float64])
		require.True(t, ok)
		require.Len(t, histogram.DataPoints, 1)

		return histogram.DataPoints[0].Count
	}

	t.Fatalf("metric %q not found", name)

	return 0
}

func metricAttributeSets(metric metricdata.Metrics) []map[string]struct{} {
	var result []map[string]struct{}

	switch data := metric.Data.(type) {
	case metricdata.Sum[int64]:
		for _, dataPoint := range data.DataPoints {
			result = append(result, attributeSet(dataPoint.Attributes.ToSlice()))
		}
	case metricdata.Histogram[float64]:
		for _, dataPoint := range data.DataPoints {
			result = append(result, attributeSet(dataPoint.Attributes.ToSlice()))
		}
	}

	return result
}

func attributeSet(attributes []attribute.KeyValue) map[string]struct{} {
	result := make(map[string]struct{}, len(attributes))
	for _, attr := range attributes {
		result[string(attr.Key)] = struct{}{}
	}

	return result
}

func attributesContain(attributes []attribute.KeyValue, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}

	actual := map[string]string{}
	for _, attr := range attributes {
		actual[string(attr.Key)] = attr.Value.AsString()
	}

	for key, value := range required {
		if actual[key] != value {
			return false
		}
	}

	return true
}
