package binary

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

const (
	testTraceID = "0af7651916cd43dd8448eb211c80319c"
	testSpanID  = "b7ad6b7169203331"
)

func TestExtractTraceparentFromFilenamePreservesFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         string
		wantSampled   bool
		wantRandom    bool
		wantTraceFlag trace.TraceFlags
	}{
		{name: "not sampled", flags: "00", wantTraceFlag: 0},
		{name: "sampled", flags: "01", wantSampled: true, wantTraceFlag: trace.FlagsSampled},
		{name: "random", flags: "02", wantRandom: true, wantTraceFlag: trace.FlagsRandom},
		{name: "sampled and random", flags: "03", wantSampled: true, wantRandom: true, wantTraceFlag: trace.FlagsSampled | trace.FlagsRandom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := "snp-x86_64.efi-00-" + testTraceID + "-" + testSpanID + "-" + tt.flags

			ctx, short, err := extractTraceparentFromFilename(context.Background(), filename)
			if err != nil {
				t.Fatalf("extractTraceparentFromFilename() error = %v", err)
			}
			if short != "snp-x86_64.efi" {
				t.Fatalf("short filename = %q, want %q", short, "snp-x86_64.efi")
			}

			sc := trace.SpanContextFromContext(ctx)
			if !sc.IsValid() {
				t.Fatal("expected a valid remote span context")
			}
			if !sc.IsRemote() {
				t.Fatal("expected extracted span context to be remote")
			}
			if sc.TraceID().String() != testTraceID {
				t.Fatalf("trace ID = %q, want %q", sc.TraceID().String(), testTraceID)
			}
			if sc.SpanID().String() != testSpanID {
				t.Fatalf("span ID = %q, want %q", sc.SpanID().String(), testSpanID)
			}
			if sc.TraceFlags() != tt.wantTraceFlag {
				t.Fatalf("trace flags = %02x, want %02x", byte(sc.TraceFlags()), byte(tt.wantTraceFlag))
			}
			if sc.IsSampled() != tt.wantSampled {
				t.Fatalf("IsSampled() = %v, want %v", sc.IsSampled(), tt.wantSampled)
			}
			if sc.IsRandom() != tt.wantRandom {
				t.Fatalf("IsRandom() = %v, want %v", sc.IsRandom(), tt.wantRandom)
			}
		})
	}
}

func TestExtractTraceparentFromFilenameRejectsInvalidTraceparent(t *testing.T) {
	original := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	})
	input := trace.ContextWithSpanContext(context.Background(), original)

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "reserved flags for version zero",
			filename: "snp-x86_64.efi-00-" + testTraceID + "-" + testSpanID + "-04",
		},
		{
			name:     "all zero trace id",
			filename: "snp-x86_64.efi-00-00000000000000000000000000000000-" + testSpanID + "-01",
		},
		{
			name:     "all zero span id",
			filename: "snp-x86_64.efi-00-" + testTraceID + "-0000000000000000-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, short, err := extractTraceparentFromFilename(input, tt.filename)
			if err == nil {
				t.Fatal("expected invalid traceparent to return an error")
			}
			if short != tt.filename {
				t.Fatalf("short filename = %q, want original %q", short, tt.filename)
			}
			if got := trace.SpanContextFromContext(ctx); !got.Equal(original) {
				t.Fatalf("span context changed on invalid traceparent: got %+v want %+v", got, original)
			}
		})
	}
}

func TestExtractTraceparentFromFilenameLeavesOrdinaryFilenameUntouched(t *testing.T) {
	original := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	})
	input := trace.ContextWithSpanContext(context.Background(), original)

	for _, filename := range []string{
		"snp-x86_64.efi",
		"snp-x86_64.efi-00-" + testTraceID + "-" + testSpanID + "-01-extra",
	} {
		ctx, short, err := extractTraceparentFromFilename(input, filename)
		if err != nil {
			t.Fatalf("extractTraceparentFromFilename(%q) error = %v", filename, err)
		}
		if short != filename {
			t.Fatalf("short filename = %q, want %q", short, filename)
		}
		if got := trace.SpanContextFromContext(ctx); !got.Equal(original) {
			t.Fatalf("span context changed for ordinary filename: got %+v want %+v", got, original)
		}
	}
}
