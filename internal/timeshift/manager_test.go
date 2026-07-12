package timeshift

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestManagerSharesOneChannelBufferAcrossLeases(t *testing.T) {
	t.Parallel()
	stream := bytes.Join([][]byte{
		testPATPacket(), testPMTPacket(), testVideoPacket(0), testFillerPacket(),
		testVideoPacket(180000), testFillerPacket(), testVideoPacket(360000),
	}, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "video/mp2t")
		_, _ = w.Write(stream)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()

	manager := NewManager(t.TempDir())
	config := Config{Enabled: true, MaxBytes: 32 << 20, MinFreeBytes: 1, MaxBuffers: 2, Window: 30 * time.Minute}
	first, err := manager.Acquire("channel-1", upstream.URL, config)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	second, err := manager.Acquire("channel-1", upstream.URL, config)
	if err != nil {
		t.Fatalf("acquire second lease: %v", err)
	}
	if first.BufferID != second.BufferID || first.ID == second.ID {
		t.Fatalf("expected shared buffer with unique leases: first=%+v second=%+v", first, second)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		status, ok := manager.Status(first.ID)
		if ok && status.SegmentCount >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("buffer did not become ready: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	manifest, ok := manager.Manifest(first.BufferID, first.ID)
	if !ok || !bytes.Contains(manifest, []byte("#EXT-X-MEDIA-SEQUENCE")) {
		t.Fatalf("expected HLS manifest, ok=%v body=%q", ok, manifest)
	}
	stats := manager.Stats()
	if stats.ActiveBuffers != 1 || stats.ActiveLeases != 2 {
		t.Fatalf("expected one buffer and two leases, got %+v", stats)
	}
	if err := manager.Clear(); err != nil {
		t.Fatalf("clear manager: %v", err)
	}
}

func TestManagerRejectsDisabledAndOverCapacityBuffers(t *testing.T) {
	t.Parallel()
	manager := NewManager(t.TempDir())
	if _, err := manager.Acquire("channel", "http://example.invalid", Config{}); err == nil {
		t.Fatal("expected disabled manager to reject acquire")
	}
	config := Config{Enabled: true, MaxBuffers: 1, MaxBytes: 1 << 20, MinFreeBytes: 1}
	if _, err := manager.Acquire("one", "http://127.0.0.1:1", config); err != nil {
		t.Fatalf("first buffer should reserve capacity: %v", err)
	}
	if _, err := manager.Acquire("two", "http://127.0.0.1:1", config); err == nil {
		t.Fatal("expected distinct channel buffer limit")
	}
	_ = manager.Clear()
}
