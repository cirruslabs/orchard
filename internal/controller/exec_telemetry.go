package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/cirruslabs/orchard/internal/opentelemetry"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	execTelemetryEventSessionStarted = "session_started"
	execTelemetryEventAttached       = "attached"
	execTelemetryEventDetached       = "detached"
	execTelemetryEventFinished       = "finished"
	execTelemetryEventFailed         = "failed"

	execTelemetryModeLegacy        = "legacy"
	execTelemetryModeReconnectable = "reconnectable"

	execTelemetryAttachmentInitial   = "initial"
	execTelemetryAttachmentReconnect = "reconnect"

	execTelemetryOutcomeSuccess  = "success"
	execTelemetryOutcomeError    = "error"
	execTelemetryOutcomeCanceled = "canceled"
	execTelemetryOutcomeClosed   = "closed"
	execTelemetryOutcomeDetached = "detached"
	execTelemetryOutcomeDropped  = "dropped"
	execTelemetryOutcomeFinished = "finished"
)

type execTelemetry struct {
	tracer trace.Tracer
	logger log.Logger

	setupAttempts    metric.Int64Counter
	sessionsStarted  metric.Int64Counter
	sessionsFinished metric.Int64Counter
	activeSessions   metric.Int64UpDownCounter
	attachments      metric.Int64Counter
	setupDuration    metric.Float64Histogram
	sessionDuration  metric.Float64Histogram

	now func() time.Time
}

func (telemetry *execTelemetry) startSetup(
	ctx context.Context,
	key execSessionKey,
	spec execSessionSpec,
) *execSetupTelemetry {
	if ctx == nil {
		ctx = context.Background()
	}

	metadata := newExecTelemetryMetadata(key, spec)
	ctx, span := telemetry.tracer.Start(ctx, "orchard.exec.setup",
		trace.WithAttributes(metadata.spanAttributes()...))

	return &execSetupTelemetry{
		telemetry: telemetry,
		metadata:  metadata,
		ctx:       ctx,
		span:      span,
		startedAt: telemetry.now(),
	}
}

func (telemetry *execTelemetry) emitLog(
	ctx context.Context,
	event string,
	metadata execTelemetryMetadata,
	extra ...log.KeyValue,
) {
	record := log.Record{}
	record.SetTimestamp(telemetry.now())
	record.SetObservedTimestamp(telemetry.now())
	record.SetSeverity(log.SeverityInfo)
	record.SetBody(log.StringValue(event))
	record.AddAttributes(metadata.logAttributes(extra...)...)
	telemetry.logger.Emit(ctx, record)
}

func newDefaultExecTelemetry() (*execTelemetry, error) {
	return newExecTelemetry(
		opentelemetry.DefaultMeter,
		opentelemetry.DefaultTracer,
		opentelemetry.DefaultLogger,
	)
}

func newExecTelemetry(
	meter metric.Meter,
	tracer trace.Tracer,
	logger log.Logger,
) (*execTelemetry, error) {
	telemetry := &execTelemetry{
		tracer: tracer,
		logger: logger,
		now:    time.Now,
	}

	var err error

	telemetry.setupAttempts, err = meter.Int64Counter("org.cirruslabs.orchard.controller.exec.setup_attempts")
	if err != nil {
		return nil, err
	}

	telemetry.sessionsStarted, err = meter.Int64Counter("org.cirruslabs.orchard.controller.exec.sessions_started")
	if err != nil {
		return nil, err
	}

	telemetry.sessionsFinished, err = meter.Int64Counter("org.cirruslabs.orchard.controller.exec.sessions_finished")
	if err != nil {
		return nil, err
	}

	telemetry.activeSessions, err = meter.Int64UpDownCounter("org.cirruslabs.orchard.controller.exec.active_sessions")
	if err != nil {
		return nil, err
	}

	telemetry.attachments, err = meter.Int64Counter("org.cirruslabs.orchard.controller.exec.attachments")
	if err != nil {
		return nil, err
	}

	telemetry.setupDuration, err = meter.Float64Histogram("org.cirruslabs.orchard.controller.exec.setup_time")
	if err != nil {
		return nil, err
	}

	telemetry.sessionDuration, err = meter.Float64Histogram("org.cirruslabs.orchard.controller.exec.session_time")
	if err != nil {
		return nil, err
	}

	return telemetry, nil
}

type execTelemetryMetadata struct {
	vmName      string
	vmUID       string
	sessionID   string
	worker      string
	mode        string
	tty         bool
	interactive bool
	commandHash string
}

func newExecTelemetryMetadata(key execSessionKey, spec execSessionSpec) execTelemetryMetadata {
	mode := execTelemetryModeLegacy
	if key.sessionID != "" {
		mode = execTelemetryModeReconnectable
	}

	return execTelemetryMetadata{
		vmName:      key.vmName,
		sessionID:   key.sessionID,
		mode:        mode,
		tty:         spec.tty,
		interactive: spec.interactive,
		commandHash: execCommandHash(spec.command),
	}
}

func (metadata *execTelemetryMetadata) setVM(vm *v1.VM) {
	metadata.vmUID = vm.UID
	metadata.worker = vm.Worker
}

func (metadata execTelemetryMetadata) spanAttributes(extra ...attribute.KeyValue) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("vm_name", metadata.vmName),
		attribute.String("vm_uid", metadata.vmUID),
		attribute.String("session_id", metadata.sessionID),
		attribute.String("mode", metadata.mode),
		attribute.Bool("tty", metadata.tty),
		attribute.Bool("interactive", metadata.interactive),
		attribute.String("worker", metadata.worker),
		attribute.String("command_sha256", metadata.commandHash),
	}

	return append(attributes, extra...)
}

func (metadata execTelemetryMetadata) metricAttributes(extra ...attribute.KeyValue) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("mode", metadata.mode),
		attribute.Bool("tty", metadata.tty),
		attribute.Bool("interactive", metadata.interactive),
	}

	return append(attributes, extra...)
}

func (metadata execTelemetryMetadata) logAttributes(extra ...log.KeyValue) []log.KeyValue {
	attributes := []log.KeyValue{
		log.String("vm_name", metadata.vmName),
		log.String("vm_uid", metadata.vmUID),
		log.String("session_id", metadata.sessionID),
		log.String("mode", metadata.mode),
		log.Bool("tty", metadata.tty),
		log.Bool("interactive", metadata.interactive),
		log.String("worker", metadata.worker),
		log.String("command_sha256", metadata.commandHash),
	}

	return append(attributes, extra...)
}

type execSetupTelemetry struct {
	telemetry *execTelemetry
	metadata  execTelemetryMetadata
	ctx       context.Context
	span      trace.Span
	startedAt time.Time

	finishOnce sync.Once
}

func (setup *execSetupTelemetry) setVM(vm *v1.VM) {
	setup.metadata.setVM(vm)
	setup.span.SetAttributes(setup.metadata.spanAttributes()...)
}

func (setup *execSetupTelemetry) startSession() (context.Context, *execSessionTelemetry) {
	sessionCtx, span := setup.telemetry.tracer.Start(
		context.WithoutCancel(setup.ctx),
		"orchard.exec.session",
		trace.WithAttributes(setup.metadata.spanAttributes()...),
	)

	return sessionCtx, &execSessionTelemetry{
		telemetry: setup.telemetry,
		metadata:  setup.metadata,
		ctx:       sessionCtx,
		span:      span,
	}
}

func (setup *execSetupTelemetry) finish(err error) {
	setup.finishOnce.Do(func() {
		outcome := execTelemetryOutcomeFromError(err)
		setup.telemetry.setupAttempts.Add(setup.ctx, 1,
			metric.WithAttributes(setup.metadata.metricAttributes(attribute.String("outcome", outcome))...))
		setup.telemetry.setupDuration.Record(setup.ctx,
			setup.telemetry.now().Sub(setup.startedAt).Seconds(),
			metric.WithAttributes(setup.metadata.metricAttributes(attribute.String("outcome", outcome))...))

		setup.span.SetAttributes(attribute.String("outcome", outcome))
		if err != nil {
			setup.span.RecordError(err)
			setup.span.SetStatus(codes.Error, err.Error())
			setup.telemetry.emitLog(setup.ctx, execTelemetryEventFailed, setup.metadata,
				log.String("outcome", outcome))
		}
		setup.span.End()
	})
}

type execSessionTelemetry struct {
	telemetry *execTelemetry
	metadata  execTelemetryMetadata
	ctx       context.Context
	span      trace.Span

	mu        sync.Mutex
	started   bool
	startedAt time.Time

	finishOnce sync.Once
}

func (session *execSessionTelemetry) start() {
	session.mu.Lock()
	if session.started {
		session.mu.Unlock()

		return
	}

	session.started = true
	session.startedAt = session.telemetry.now()
	session.mu.Unlock()

	session.telemetry.sessionsStarted.Add(session.ctx, 1,
		metric.WithAttributes(session.metadata.metricAttributes()...))
	session.telemetry.activeSessions.Add(session.ctx, 1,
		metric.WithAttributes(session.metadata.metricAttributes()...))
	session.telemetry.emitLog(session.ctx, execTelemetryEventSessionStarted, session.metadata)
}

func (session *execSessionTelemetry) attach(
	requestCtx context.Context,
	kind string,
) *execAttachmentTelemetry {
	var links []trace.Link
	if spanContext := trace.SpanContextFromContext(requestCtx); spanContext.IsValid() {
		links = append(links, trace.Link{SpanContext: spanContext})
	}

	ctx, span := session.telemetry.tracer.Start(
		session.ctx,
		"orchard.exec.attachment",
		trace.WithAttributes(session.metadata.spanAttributes(attribute.String("attachment_kind", kind))...),
		trace.WithLinks(links...),
	)

	session.telemetry.attachments.Add(ctx, 1,
		metric.WithAttributes(session.metadata.metricAttributes(attribute.String("attachment_kind", kind))...))
	session.telemetry.emitLog(ctx, execTelemetryEventAttached, session.metadata,
		log.String("attachment_kind", kind))

	return &execAttachmentTelemetry{
		session: session,
		ctx:     ctx,
		span:    span,
		kind:    kind,
	}
}

func (session *execSessionTelemetry) fail(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}

	session.telemetry.emitLog(session.ctx, execTelemetryEventFailed, session.metadata,
		log.String("outcome", execTelemetryOutcomeError))
}

func (session *execSessionTelemetry) finish(outcome string, err error) {
	session.finishOnce.Do(func() {
		session.mu.Lock()
		started := session.started
		startedAt := session.startedAt
		session.mu.Unlock()

		session.span.SetAttributes(attribute.String("outcome", outcome))
		if err != nil && !errors.Is(err, context.Canceled) {
			session.span.RecordError(err)
			session.span.SetStatus(codes.Error, err.Error())
		}

		if started {
			session.telemetry.sessionsFinished.Add(session.ctx, 1,
				metric.WithAttributes(session.metadata.metricAttributes(attribute.String("outcome", outcome))...))
			session.telemetry.activeSessions.Add(session.ctx, -1,
				metric.WithAttributes(session.metadata.metricAttributes()...))
			session.telemetry.sessionDuration.Record(session.ctx,
				session.telemetry.now().Sub(startedAt).Seconds(),
				metric.WithAttributes(session.metadata.metricAttributes(attribute.String("outcome", outcome))...))
			session.telemetry.emitLog(session.ctx, execTelemetryEventFinished, session.metadata,
				log.String("outcome", outcome))
		}

		session.span.End()
	})
}

type execAttachmentTelemetry struct {
	session *execSessionTelemetry
	ctx     context.Context
	span    trace.Span
	kind    string

	finishOnce sync.Once
}

func (attachment *execAttachmentTelemetry) finish(outcome string) {
	attachment.finishOnce.Do(func() {
		attachment.span.SetAttributes(attribute.String("outcome", outcome))
		if outcome != execTelemetryOutcomeFinished {
			attachment.session.telemetry.emitLog(attachment.ctx, execTelemetryEventDetached,
				attachment.session.metadata,
				log.String("attachment_kind", attachment.kind),
				log.String("outcome", outcome))
		}
		attachment.span.End()
	})
}

func execCommandHash(command string) string {
	sum := sha256.Sum256([]byte(command))

	return hex.EncodeToString(sum[:])
}

func execTelemetryOutcomeFromError(err error) string {
	switch {
	case err == nil:
		return execTelemetryOutcomeSuccess
	case errors.Is(err, context.Canceled):
		return execTelemetryOutcomeCanceled
	default:
		return execTelemetryOutcomeError
	}
}
