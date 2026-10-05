package whatsapp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
)

// SAYWHAT-PATCH: webhook-client-pool — retain one bounded pool per TLS policy.
var webhookClients = map[bool]*http.Client{
	false: newWebhookClient(false),
	true:  newWebhookClient(true),
}

func newWebhookClient(insecureSkipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureSkipVerify}
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 2
	transport.IdleConnTimeout = 30 * time.Second
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}
}

func submitWebhook(ctx context.Context, payload map[string]any, url string, webhookConfig *chatstorage.DeviceWebhookConfig) error {
	// Determine effective config - use device-specific if set, otherwise fall back to global
	insecureSkipVerify := config.WhatsappWebhookInsecureSkipVerify
	webhookSecret := config.WhatsappWebhookSecret

	if webhookConfig != nil {
		if webhookConfig.WebhookInsecureSkipVerify {
			insecureSkipVerify = true
		}
		if webhookConfig.WebhookSecret != "" {
			webhookSecret = webhookConfig.WebhookSecret
		}
	}

	// SAYWHAT-PATCH: webhook-client-pool — device TLS policies never share a pool.
	client := webhookClients[insecureSkipVerify]

	postBody, err := json.Marshal(payload)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("Failed to marshal body: %v", err))
	}

	// SAYWHAT-PATCH: webhook-content-length — preserve framing on every attempt.
	// Pass the body to NewRequestWithContext so it sets ContentLength and GetBody.
	// Building the request with a nil body makes Go fall back to chunked transfer
	// encoding with no Content-Length, which some receivers (notably PHP reading
	// php://input behind nginx/FPM) deliver to the application as an empty body.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(postBody))
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("error when create http object %v", err))
	}

	secretKey := []byte(webhookSecret)
	signature, err := utils.GetMessageDigestOrSignature(postBody, secretKey)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("error when create signature %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%s", signature))

	var attempt int
	var maxAttempts = 5
	var sleepDuration = 1 * time.Second

	for attempt = 0; attempt < maxAttempts; attempt++ {
		// SAYWHAT-PATCH: webhook-content-length — retries keep the full body.
		// Rewind the body for each attempt. GetBody returns a fresh reader while
		// leaving ContentLength intact, unlike assigning req.Body directly.
		body, err := req.GetBody()
		if err != nil {
			return pkgError.WebhookError(fmt.Sprintf("error when rewind body %v", err))
		}
		req.Body = body

		resp, err := client.Do(req)
		if err == nil {
			// SAYWHAT-PATCH: webhook-client-pool — finish responses before retrying.
			// Small responses reach EOF for reuse; large ones are closed without reuse.
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				logrus.Infof("Successfully submitted webhook on attempt %d", attempt+1)
				return nil
			}
			err = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}
		logrus.Warnf("Attempt %d to submit webhook failed: %v", attempt+1, err)
		if attempt < maxAttempts-1 {
			time.Sleep(sleepDuration)
			sleepDuration *= 2
		}
	}

	return pkgError.WebhookError(fmt.Sprintf("error when submit webhook after %d attempts: %v", attempt, err))
}
