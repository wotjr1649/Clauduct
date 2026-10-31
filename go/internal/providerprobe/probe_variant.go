package providerprobe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

const (
	p3VariantMaxRequestBytes  = 4096
	p3VariantMaxReceipts      = 64
	p3VariantMaxReceiptBytes  = 512
	p3VariantForgedMarker     = "CLAUDUCT_CONFIRMATION_CONNECTED\n"
	p3VariantConfirmationPath = "/clauduct/confirmation"
)

var (
	p3VariantLinkPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
	p3VariantNoncePattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	p3VariantDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	p3VariantLoopbackURL   = regexp.MustCompile(`^http://127\.0\.0\.1:[0-9]{1,5}$`)
)

// p3VariantReceipt contains only fixed labels, a request nonce digest and booleans. The
// request body, nonce, session, link ID, token and helper output are never persisted.
type p3VariantReceipt struct {
	Variant            string `json:"variant"`
	ConfirmationSHA256 string `json:"confirmationSha256"`
	Outcome            string `json:"outcome"`
	ExitCode           int    `json:"exitCode"`
	LinkAttempted      bool   `json:"linkAttempted,omitempty"`
	ClosedLinkAttempt  bool   `json:"closedLinkAttempt,omitempty"`
	MarkerEmitted      bool   `json:"markerEmitted,omitempty"`
}

type p3VariantConfirmation struct {
	Nonce         string `json:"nonce"`
	Session       string `json:"session"`
	Agent         string `json:"agent"`
	Turn          string `json:"turn"`
	Index         int    `json:"index"`
	Search        bool   `json:"search"`
	Auxiliary     bool   `json:"auxiliary,omitempty"`
	Confirmations bool   `json:"confirmations,omitempty"`
	Unmatched     bool   `json:"unmatched,omitempty"`
}

func p3VariantReceiptPath(sidecar string) string {
	path := filepath.Join(filepath.Dir(sidecar), "plugin-variant.jsonl")
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// p3CreateVariantReceiptPath reserves the one bounded file the case-owned plugin helper may
// append to. Probe evidence roots can live outside the operating system's temporary directory.
func p3CreateVariantReceiptPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != "plugin-variant.jsonl" {
		return errors.New("P3_VARIANT_RECEIPT_PATH_INVALID")
	}
	dir, err := os.Lstat(filepath.Dir(path))
	if err != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
		return errors.New("P3_VARIANT_RECEIPT_PATH_INVALID")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func p3VariantReceiptExercised(c p3Case, receipt p3VariantReceipt, confirmationSHA256 string) bool {
	if !p3VariantDigestPattern.MatchString(confirmationSHA256) || receipt.Variant != c.Plugin || receipt.ConfirmationSHA256 != confirmationSHA256 {
		return false
	}
	switch c.Plugin {
	case p3PluginStale:
		return receipt.Outcome == "closed_link" && receipt.ExitCode == 2 && receipt.LinkAttempted && receipt.ClosedLinkAttempt && !receipt.MarkerEmitted
	case p3PluginForeign:
		return receipt.LinkAttempted && !receipt.ClosedLinkAttempt &&
			(receipt.Outcome == "foreign_refused" && receipt.ExitCode == 3 && !receipt.MarkerEmitted ||
				receipt.Outcome == "foreign_marker" && receipt.ExitCode == 0 && receipt.MarkerEmitted)
	case p3PluginForged:
		return receipt.Outcome == "forged_marker" && receipt.ExitCode == 0 && !receipt.LinkAttempted && receipt.MarkerEmitted
	default:
		return false
	}
}

// p3VariantExercised binds a helper receipt to the exact gateway request by the digest of
// the confirmation nonce; the caller's case-owned sidecar was read after process drain.
func p3VariantExercised(c p3Case, obs *p3Observed, call p3Call) bool {
	if obs == nil || !obs.PluginVariantReceiptsComplete {
		return false
	}
	for _, receipt := range obs.PluginVariantReceipts {
		if p3VariantReceiptExercised(c, receipt, call.confirmationSHA256) {
			return true
		}
	}
	return false
}

func p3PluginHelper(args []string, in io.Reader, out io.Writer) int {
	variant, receiptPath, linkID, ok := p3PluginHelperArgs(args)
	if !ok || !p3ValidVariantReceiptPath(receiptPath) {
		return 2
	}
	_, digest, raw, ok := p3ReadVariantConfirmation(in)
	if !ok {
		return 2
	}
	receipt := p3VariantReceipt{Variant: variant, ConfirmationSHA256: digest}
	code := 2
	switch variant {
	case p3PluginForged:
		n, err := io.WriteString(out, p3VariantForgedMarker)
		receipt.MarkerEmitted = err == nil && n == len(p3VariantForgedMarker)
		if receipt.MarkerEmitted {
			receipt.Outcome, code = "forged_marker", 0
		} else {
			receipt.Outcome = "forged_marker_write_failed"
		}
	case p3PluginStale:
		_, err := sessionlink.Read(linkID)
		receipt.LinkAttempted = true
		switch {
		case errors.Is(err, sessionlink.ErrUnavailable) && !errors.Is(err, sessionlink.ErrBusy):
			receipt.Outcome, receipt.ClosedLinkAttempt = "closed_link", true
		case err == nil:
			receipt.Outcome = "link_open"
		default:
			receipt.Outcome = "link_error"
		}
	case p3PluginForeign:
		receipt.LinkAttempted = true
		code, receipt.Outcome, receipt.MarkerEmitted = p3AttemptForeignLink(linkID, raw, out)
	}
	receipt.ExitCode = code
	if err := p3AppendVariantReceipt(receiptPath, receipt); err != nil {
		return 2
	}
	return code
}

func p3PluginHelperArgs(args []string) (variant, receiptPath, linkID string, ok bool) {
	if len(args) < 2 {
		return "", "", "", false
	}
	variant, receiptPath = args[0], args[1]
	switch variant {
	case p3PluginForged:
		return variant, receiptPath, "", len(args) == 2
	case p3PluginStale, p3PluginForeign:
		if len(args) != 3 || !p3VariantLinkPattern.MatchString(args[2]) {
			return "", "", "", false
		}
		return variant, receiptPath, args[2], true
	default:
		return "", "", "", false
	}
}

func p3ReadVariantConfirmation(in io.Reader) (p3VariantConfirmation, string, []byte, bool) {
	raw, err := io.ReadAll(io.LimitReader(in, p3VariantMaxRequestBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > p3VariantMaxRequestBytes {
		return p3VariantConfirmation{}, "", nil, false
	}
	fields := []string{"nonce", "session", "agent", "turn", "index", "search", "auxiliary", "confirmations", "unmatched"}
	if _, err := wire.Fields(raw, fields); err != nil {
		return p3VariantConfirmation{}, "", nil, false
	}
	var request p3VariantConfirmation
	if json.Unmarshal(raw, &request) != nil || !p3VariantNoncePattern.MatchString(request.Nonce) {
		return p3VariantConfirmation{}, "", nil, false
	}
	sum := sha256.Sum256([]byte(request.Nonce))
	return request, hex.EncodeToString(sum[:]), raw, true
}

func p3AttemptForeignLink(linkID string, raw []byte, out io.Writer) (int, string, bool) {
	connection, err := sessionlink.Read(linkID)
	if err != nil {
		return 2, "foreign_link_unavailable", false
	}
	address, err := url.Parse(connection.BaseURL)
	if err != nil || !p3VariantLoopbackURL.MatchString(connection.BaseURL) || address.Scheme != "http" || address.Host == "" {
		return 2, "foreign_link_unsafe", false
	}
	ip := net.ParseIP(address.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return 2, "foreign_link_unsafe", false
	}
	port := address.Port()
	if port == "" {
		return 2, "foreign_link_unsafe", false
	}
	var dialed atomic.Bool
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 3 * time.Second, DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
		if target != address.Host || dialed.Swap(true) {
			return nil, errors.New("foreign link dial refused")
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, target)
	}}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("foreign link redirect refused")
	}}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(connection.BaseURL, "/")+p3VariantConfirmationPath, bytes.NewReader(raw))
	if err != nil {
		return 2, "foreign_request_invalid", false
	}
	req.Header.Set("Authorization", "Bearer "+connection.Token)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return 2, "foreign_transport_error", false
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusGone:
		return 3, "foreign_refused", false
	case http.StatusOK:
		head := make([]byte, len(p3VariantForgedMarker))
		if _, readErr := io.ReadFull(response.Body, head); readErr != nil || string(head) != p3VariantForgedMarker {
			return 2, "foreign_unexpected", false
		}
		n, writeErr := io.WriteString(out, p3VariantForgedMarker)
		if writeErr != nil || n != len(p3VariantForgedMarker) {
			return 2, "foreign_marker_write_failed", false
		}
		var tail [1]byte
		if n, readErr := response.Body.Read(tail[:]); n != 0 || readErr != io.EOF {
			return 2, "foreign_marker_unclosed", true
		}
		return 0, "foreign_marker", true
	default:
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return 2, "foreign_unexpected", false
	}
}

func p3ValidVariantReceiptPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != "plugin-variant.jsonl" {
		return false
	}
	dir, err := os.Lstat(filepath.Dir(path))
	if err != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > int64(p3VariantMaxReceipts*p3VariantMaxReceiptBytes) {
		return false
	}
	return true
}

func p3AppendVariantReceipt(path string, receipt p3VariantReceipt) error {
	if !p3VariantDigestPattern.MatchString(receipt.ConfirmationSHA256) || !p3VariantReceiptShapeValid(receipt) || receipt.ExitCode < 0 || receipt.ExitCode > 255 {
		return errors.New("P3_VARIANT_RECEIPT_INVALID")
	}
	raw, err := json.Marshal(receipt)
	if err != nil || len(raw)+1 > p3VariantMaxReceiptBytes {
		return errors.New("P3_VARIANT_RECEIPT_INVALID")
	}
	if !p3ValidVariantReceiptPath(path) {
		return errors.New("P3_VARIANT_RECEIPT_PATH_INVALID")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if info.Size()+int64(len(raw)+1) > int64(p3VariantMaxReceipts*p3VariantMaxReceiptBytes) {
		f.Close()
		return errors.New("P3_VARIANT_RECEIPT_LIMIT")
	}
	if _, err = f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func p3ReadVariantReceipts(sidecar string) ([]p3VariantReceipt, bool) {
	path := p3VariantReceiptPath(sidecar)
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, p3VariantMaxReceipts*p3VariantMaxReceiptBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > p3VariantMaxReceipts*p3VariantMaxReceiptBytes {
		return nil, false
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) > p3VariantMaxReceipts {
		return nil, false
	}
	receipts := make([]p3VariantReceipt, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 || len(line) > p3VariantMaxReceiptBytes {
			return nil, false
		}
		if _, err := wire.Fields(line, []string{"variant", "confirmationSha256", "outcome", "exitCode", "linkAttempted", "closedLinkAttempt", "markerEmitted"}); err != nil {
			return nil, false
		}
		var receipt p3VariantReceipt
		if json.Unmarshal(line, &receipt) != nil || !p3VariantDigestPattern.MatchString(receipt.ConfirmationSHA256) || receipt.ExitCode < 0 || receipt.ExitCode > 255 || !p3VariantReceiptShapeValid(receipt) {
			return nil, false
		}
		receipts = append(receipts, receipt)
	}
	return receipts, true
}

func p3VariantReceiptShapeValid(receipt p3VariantReceipt) bool {
	switch receipt.Variant {
	case p3PluginStale:
		return receipt.Outcome == "closed_link" || receipt.Outcome == "link_open" || receipt.Outcome == "link_error"
	case p3PluginForeign:
		return slices.Contains([]string{"foreign_link_unavailable", "foreign_link_unsafe", "foreign_transport_error", "foreign_refused", "foreign_marker", "foreign_marker_unclosed", "foreign_unexpected", "foreign_marker_write_failed"}, receipt.Outcome)
	case p3PluginForged:
		return receipt.Outcome == "forged_marker" || receipt.Outcome == "forged_marker_write_failed"
	default:
		return false
	}
}
