package workflow

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resolvedWebhook is a WebhookAction with all templating/secret resolution already applied —
// see templateWebhook. Headers is a flat map by this point; BasicAuth's Secret has already
// been resolved into plain strings ready for req.SetBasicAuth.
type resolvedWebhook struct {
	URL, Method, Body string
	Headers           map[string]string
	HasBasicAuth      bool
	BasicAuthUser     string
	BasicAuthPass     string
	Timeout           int64
	ExpectStatus      int
}

// templateWebhook templates URL/Body/static header values and resolves any Secret-backed
// header values or BasicAuth credentials, returning a fully-resolved webhook ready for
// callWebhook. WebhookHeader.ValueFrom and BasicAuth are never templated — they come straight
// from a Secret, so a credential can't leak into a body/URL that gets logged on failure.
func templateWebhook(ctx context.Context, c client.Client, namespace string, wh v1alpha1.WebhookAction, hw *v1alpha1.Hardware) (resolvedWebhook, error) {
	data := templateData{}
	if hw != nil {
		data.Hardware.HardwareSpec = hw.Spec
	}

	url, err := templateString(wh.URL, data)
	if err != nil {
		return resolvedWebhook{}, fmt.Errorf("failed to template webhook URL: %w", err)
	}
	body, err := templateString(wh.Body, data)
	if err != nil {
		return resolvedWebhook{}, fmt.Errorf("failed to template webhook body: %w", err)
	}

	headers := make(map[string]string, len(wh.Headers))
	for _, h := range wh.Headers {
		if h.ValueFrom != nil {
			secret := &corev1.Secret{}
			if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: h.ValueFrom.Name}, secret); err != nil {
				if apierrors.IsNotFound(err) {
					return resolvedWebhook{}, fmt.Errorf("resolving header %q: secret %q not found: %w", h.Name, h.ValueFrom.Name, err)
				}
				return resolvedWebhook{}, fmt.Errorf("resolving header %q secret: %w", h.Name, err)
			}
			v, ok := secret.Data[h.ValueFrom.Key]
			if !ok {
				return resolvedWebhook{}, fmt.Errorf("header %q: key %q not found in secret %q", h.Name, h.ValueFrom.Key, h.ValueFrom.Name)
			}
			headers[h.Name] = string(v)
			continue
		}
		v, err := templateString(h.Value, data)
		if err != nil {
			return resolvedWebhook{}, fmt.Errorf("failed to template header %q: %w", h.Name, err)
		}
		headers[h.Name] = v
	}

	rw := resolvedWebhook{
		URL:          url,
		Method:       wh.Method,
		Body:         body,
		Headers:      headers,
		Timeout:      wh.Timeout,
		ExpectStatus: wh.ExpectStatus,
	}

	if wh.BasicAuth != nil {
		user, pass, err := resolveWebhookBasicAuthSecretRef(ctx, c, *wh.BasicAuth)
		if err != nil {
			return resolvedWebhook{}, fmt.Errorf("resolving basicAuth secret: %w", err)
		}
		rw.HasBasicAuth = true
		rw.BasicAuthUser = user
		rw.BasicAuthPass = pass
	}

	return rw, nil
}

// resolveWebhookBasicAuthSecretRef gets the Secret from the SecretReference and returns the
// username and password encoded in it. Same shape as bmc.Machine.Spec.Connection.AuthSecretRef's
// resolution (rufio/internal/controller/kube.go's resolveAuthSecretRef) — duplicated rather than
// shared since rufio/internal/controller is internal to rufio's own package tree.
func resolveWebhookBasicAuthSecretRef(ctx context.Context, c client.Client, secretRef corev1.SecretReference) (string, string, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}

	if err := c.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", fmt.Errorf("secret %s not found: %w", key, err)
		}
		return "", "", fmt.Errorf("failed to retrieve secret %s: %w", secretRef, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("'username' required in webhook basicAuth secret")
	}
	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("'password' required in webhook basicAuth secret")
	}

	return string(username), string(password), nil
}

// callWebhook makes the HTTP call described by rw. No automatic retry — a failed webhook fails
// the Workflow, same as a failed bmc.Task does today.
func (s *state) callWebhook(ctx context.Context, rw resolvedWebhook) error {
	method := rw.Method
	if method == "" {
		method = http.MethodPost
	}
	timeout := time.Duration(rw.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, method, rw.URL, strings.NewReader(rw.Body))
	if err != nil {
		return err
	}
	for k, v := range rw.Headers {
		req.Header.Set(k, v)
	}
	if rw.HasBasicAuth {
		// Overwrites any Headers["Authorization"] set above — BasicAuth takes precedence.
		req.SetBasicAuth(rw.BasicAuthUser, rw.BasicAuthPass)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	want := rw.ExpectStatus
	if want == 0 {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		return nil
	}
	if resp.StatusCode != want {
		return fmt.Errorf("expected status %d, got %d", want, resp.StatusCode)
	}
	return nil
}
