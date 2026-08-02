package diagnostic

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxBufferedEvents = 256
	maxEventLength    = 8192
	maxFieldLength    = 2048
)

var traceSequence atomic.Uint64

type traceContextKey struct{}

// Field is a structured diagnostic log field
type Field struct {
	Key   string
	Value any
}

// Value creates a structured diagnostic log field
func Value(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// Trace buffers request events until failure or emits them immediately in verbose mode
type Trace struct {
	mu                 sync.Mutex
	id                 string
	request            string
	startedAt          time.Time
	verbose            bool
	finished           bool
	environmentEmitted bool
	environment        map[string]any
	events             []string
	droppedEvents      int
	emit               func(string)
}

// New creates a request diagnostic trace
func New(request string, verbose bool, emit func(string), environment ...Field) *Trace {
	if emit == nil {
		emit = func(message string) {
			log.Print(message)
		}
	}
	startedAt := time.Now()
	trace := &Trace{
		id:          fmt.Sprintf("%d-%d", startedAt.UnixNano(), traceSequence.Add(1)),
		request:     strings.TrimSpace(request),
		startedAt:   startedAt,
		verbose:     verbose,
		environment: make(map[string]any, len(environment)),
		events:      make([]string, 0, maxBufferedEvents),
		emit:        emit,
	}
	trace.setEnvironmentLocked(environment...)
	if verbose {
		trace.environmentEmitted = true
		trace.emit(trace.environmentEntryLocked("snapshot"))
	}
	return trace
}

// WithTrace returns a context carrying trace
func WithTrace(ctx context.Context, trace *Trace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey{}, trace)
}

// FromContext returns the diagnostic trace carried by ctx
func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceContextKey{}).(*Trace)
	return trace
}

// SetVerbose enables immediate event emission and flushes events buffered so far
func (t *Trace) SetVerbose(verbose bool) {
	if t == nil || !verbose {
		return
	}

	t.mu.Lock()
	if t.finished || t.verbose {
		t.mu.Unlock()
		return
	}
	t.verbose = true
	environmentEntry := ""
	if !t.environmentEmitted {
		t.environmentEmitted = true
		environmentEntry = t.environmentEntryLocked("snapshot")
	}
	events := append([]string(nil), t.events...)
	t.events = nil
	t.mu.Unlock()

	if environmentEntry != "" {
		t.emit(environmentEntry)
	}
	for _, entry := range events {
		t.emit(entry)
	}
}

// SetEnvironment adds or replaces fields included in failure and verbose logs
func (t *Trace) SetEnvironment(fields ...Field) {
	if t == nil || len(fields) == 0 {
		return
	}

	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.setEnvironmentLocked(fields...)
	verbose := t.verbose
	entry := ""
	if verbose {
		entry = t.entryLocked([]Field{
			Value("stage", "environment"),
			Value("event", "update"),
		}, fields...)
	}
	t.mu.Unlock()

	if entry != "" {
		t.emit(entry)
	}
}

// Event records a structured stage event
func (t *Trace) Event(stage string, event string, fields ...Field) {
	if t == nil {
		return
	}
	prefix := []Field{
		Value("stage", stage),
		Value("event", event),
	}
	t.record(t.entry(prefix, fields...))
}

// Message records an event fragment produced by an existing diagnostic boundary
func (t *Trace) Message(message string, fields ...Field) {
	if t == nil {
		return
	}
	entry := t.entry(fields, nil...)
	message = strings.TrimSpace(strings.ToValidUTF8(message, "?"))
	if message != "" {
		entry += " " + message
	}
	t.record(limitString(entry, maxEventLength))
}

// Finish completes the trace, discarding successful buffered traces and flushing failed ones
func (t *Trace) Finish(err error) {
	if t == nil {
		return
	}

	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	durationMS := time.Since(t.startedAt).Milliseconds()
	if t.verbose {
		fields := []Field{
			Value("event", "complete"),
			Value("outcome", outcome(err)),
			Value("duration_ms", durationMS),
		}
		if err != nil {
			fields = append(fields, Value("error", err))
		}
		entry := t.entryLocked(fields)
		t.mu.Unlock()
		t.emit(entry)
		return
	}
	if err == nil {
		t.events = nil
		t.mu.Unlock()
		return
	}

	events := append([]string(nil), t.events...)
	summaryFields := []Field{
		Value("event", "failure"),
		Value("duration_ms", durationMS),
		Value("buffered_events", len(events)),
		Value("dropped_events", t.droppedEvents),
	}
	summary := t.entryLocked(summaryFields) + " " + formatEnvironment(t.environment) + " " + formatField(Value("error", err))
	t.events = nil
	t.mu.Unlock()

	t.emit(limitString(strings.TrimSpace(summary), maxEventLength))
	for _, entry := range events {
		t.emit(entry)
	}
}

func (t *Trace) record(entry string) {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	if t.verbose {
		t.mu.Unlock()
		t.emit(entry)
		return
	}
	if len(t.events) == maxBufferedEvents {
		copy(t.events, t.events[1:])
		t.events[len(t.events)-1] = entry
		t.droppedEvents++
		t.mu.Unlock()
		return
	}
	t.events = append(t.events, entry)
	t.mu.Unlock()
}

func (t *Trace) entry(fields []Field, extra ...Field) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.entryLocked(fields, extra...)
}

func (t *Trace) entryLocked(fields []Field, extra ...Field) string {
	allFields := make([]Field, 0, 2+len(fields)+len(extra))
	allFields = append(allFields,
		Value("request_id", t.id),
		Value("request", t.request),
	)
	allFields = append(allFields, fields...)
	allFields = append(allFields, extra...)
	return limitString("nexus diagnostic "+formatFields(allFields), maxEventLength)
}

func (t *Trace) environmentEntryLocked(event string) string {
	entry := t.entryLocked([]Field{
		Value("stage", "environment"),
		Value("event", event),
	})
	environment := formatEnvironment(t.environment)
	if environment != "" {
		entry += " " + environment
	}
	return limitString(entry, maxEventLength)
}

func (t *Trace) setEnvironmentLocked(fields ...Field) {
	for _, field := range fields {
		key := sanitizeKey(field.Key)
		if key == "" {
			continue
		}
		t.environment[key] = field.Value
	}
}

func formatEnvironment(environment map[string]any) string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]Field, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, Value(key, environment[key]))
	}
	return formatFields(fields)
}

func formatFields(fields []Field) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		formatted := formatField(field)
		if formatted != "" {
			parts = append(parts, formatted)
		}
	}
	return strings.Join(parts, " ")
}

func formatField(field Field) string {
	key := sanitizeKey(field.Key)
	if key == "" {
		return ""
	}
	switch value := field.Value.(type) {
	case nil:
		return key + "=null"
	case string:
		return key + "=" + strconv.Quote(limitString(strings.ToValidUTF8(value, "?"), maxFieldLength))
	case error:
		return key + "=" + strconv.Quote(limitString(strings.ToValidUTF8(value.Error(), "?"), maxFieldLength))
	case bool:
		return key + "=" + strconv.FormatBool(value)
	case int:
		return key + "=" + strconv.Itoa(value)
	case int64:
		return key + "=" + strconv.FormatInt(value, 10)
	case uint64:
		return key + "=" + strconv.FormatUint(value, 10)
	default:
		return key + "=" + strconv.Quote(limitString(fmt.Sprint(value), maxFieldLength))
	}
}

func sanitizeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	var result strings.Builder
	result.Grow(len(key))
	for _, char := range key {
		switch {
		case char >= 'a' && char <= 'z':
			result.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			result.WriteRune(char)
		case char >= '0' && char <= '9':
			result.WriteRune(char)
		case char == '_', char == '-':
			result.WriteRune(char)
		default:
			result.WriteByte('_')
		}
	}
	return result.String()
}

func limitString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func outcome(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}
