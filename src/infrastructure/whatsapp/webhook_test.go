package whatsapp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
)

type capturedRequest struct {
	contentLength    int64
	transferEncoding []string
	body             string
}

// SAYWHAT-PATCH: webhook-client-pool — assert socket reuse, including retries.
func TestSubmitWebhookReusesConnections(t *testing.T) {
	previous := config.WhatsappWebhookInsecureSkipVerify
	config.WhatsappWebhookInsecureSkipVerify = false
	t.Cleanup(func() { config.WhatsappWebhookInsecureSkipVerify = previous })
	var attempts atomic.Int32
	var addresses sync.Map
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		addresses.Store(request.RemoteAddr, true)
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = writer.Write([]byte("response body must be drained"))
	}))
	defer server.Close()
	defer webhookClients[false].CloseIdleConnections()
	for delivery := 0; delivery < 20; delivery++ {
		if err := submitWebhook(context.Background(), map[string]any{"event": "test"}, server.URL, nil); err != nil {
			t.Fatal(err)
		}
	}
	connections := 0
	addresses.Range(func(_, _ any) bool { connections++; return true })
	if connections != 1 || attempts.Load() != 21 {
		t.Fatalf("got %d connections for %d attempts, want 1 for 21", connections, attempts.Load())
	}
}

// SAYWHAT-PATCH: webhook-client-pool — one device cannot weaken another's TLS.
func TestWebhookClientsKeepTLSPoliciesSeparate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()
	defer webhookClients[true].CloseIdleConnections()
	previous := config.WhatsappWebhookInsecureSkipVerify
	config.WhatsappWebhookInsecureSkipVerify = false
	t.Cleanup(func() { config.WhatsappWebhookInsecureSkipVerify = previous })
	if err := submitWebhook(context.Background(), map[string]any{}, server.URL, &chatstorage.DeviceWebhookConfig{WebhookInsecureSkipVerify: true}); err != nil {
		t.Fatal(err)
	}
	response, err := webhookClients[false].Get(server.URL)
	if err == nil {
		response.Body.Close()
		t.Fatal("verified client accepted a self-signed certificate after an insecure device delivery")
	}
}

// startCapturingWebhookServer returns a server that records every request it
// receives and answers with the given status codes in order (the last status is
// reused once the list is exhausted).
func startCapturingWebhookServer(t *testing.T, statuses ...int) (*httptest.Server, *[]capturedRequest) {
	t.Helper()

	var mu sync.Mutex
	captured := make([]capturedRequest, 0, len(statuses))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}

		mu.Lock()
		idx := len(captured)
		captured = append(captured, capturedRequest{
			contentLength:    r.ContentLength,
			transferEncoding: r.TransferEncoding,
			body:             string(body),
		})
		mu.Unlock()

		status := statuses[len(statuses)-1]
		if idx < len(statuses) {
			status = statuses[idx]
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv, &captured
}

func TestSubmitWebhookSendsContentLength(t *testing.T) {
	srv, captured := startCapturingWebhookServer(t, http.StatusOK)

	payload := map[string]any{"event": "message", "body": "hello"}
	if err := submitWebhook(context.Background(), payload, srv.URL, nil); err != nil {
		t.Fatalf("submitWebhook returned error: %v", err)
	}

	reqs := *captured
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	got := reqs[0]
	if got.body == "" {
		t.Fatal("receiver got an empty body")
	}
	if got.contentLength != int64(len(got.body)) {
		t.Errorf("Content-Length = %d, want %d (receivers relying on Content-Length see an empty body without it)",
			got.contentLength, len(got.body))
	}
	if len(got.transferEncoding) != 0 {
		t.Errorf("Transfer-Encoding = %v, want none (chunked framing breaks some receivers)", got.transferEncoding)
	}
}

func TestSubmitWebhookRetryKeepsFullBody(t *testing.T) {
	srv, captured := startCapturingWebhookServer(t, http.StatusInternalServerError, http.StatusOK)

	payload := map[string]any{"event": "message", "body": "retry me"}
	if err := submitWebhook(context.Background(), payload, srv.URL, nil); err != nil {
		t.Fatalf("submitWebhook returned error: %v", err)
	}

	reqs := *captured
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (1 failure + 1 retry), got %d", len(reqs))
	}

	if reqs[1].body != reqs[0].body {
		t.Errorf("retry body = %q, want %q (the body must be rewound between attempts)", reqs[1].body, reqs[0].body)
	}
	if reqs[1].contentLength != int64(len(reqs[1].body)) {
		t.Errorf("retry Content-Length = %d, want %d", reqs[1].contentLength, len(reqs[1].body))
	}
}
