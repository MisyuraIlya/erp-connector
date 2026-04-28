package hasavshevet

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"erp-connector/internal/config"
	"erp-connector/internal/email"
	"erp-connector/internal/logger"
	"erp-connector/internal/pdf"
	"erp-connector/internal/print"
)

// connectorVersion is reported as the User-Agent suffix to the backend so the
// admin can distinguish connector instances in token usage telemetry.
const connectorVersion = "0.1.0"

// PDFPostOrderHook generates, prints, and/or emails a PDF invoice
// after each successful order. Implements PostOrderHook.
type PDFPostOrderHook struct {
	cfg       config.Config
	pdfGen    *pdf.Generator
	emailSend *email.Sender // nil if email not configured
	log       logger.LoggerService
}

// NewPDFPostOrderHook creates a hook that handles post-order PDF operations.
func NewPDFPostOrderHook(cfg config.Config, pdfGen *pdf.Generator, emailSend *email.Sender, log logger.LoggerService) *PDFPostOrderHook {
	return &PDFPostOrderHook{
		cfg:       cfg,
		pdfGen:    pdfGen,
		emailSend: emailSend,
		log:       log,
	}
}

// AfterOrder generates a PDF from the order data, optionally prints it and/or emails it.
// When `cfg.PDF.UseRemoteTemplate` is true and a token is configured for this
// documentType, the connector fetches a pre-rendered HTML document from the
// backend and converts it to PDF; otherwise it falls back to the embedded
// invoice template. Any failure here is non-fatal — the order itself has
// already been written to the ERP successfully by the caller.
func (h *PDFPostOrderHook) AfterOrder(ctx context.Context, req OrderRequest, result *OrderResult) error {
	orderNum := fmt.Sprintf("%d", result.OrderNumber)

	if h.cfg.PDF.UseRemoteTemplate {
		token := lookupRemoteToken(h.cfg.PDF.RemoteTokens, req.DocumentType)
		if token == "" {
			h.log.Warn(fmt.Sprintf(
				"remote template enabled but no token configured for documentType=%s; falling back to local template",
				req.DocumentType,
			))
		} else {
			pdfBytes, err := h.fetchRemoteHTMLAndRenderPDF(ctx, token, req)
			if err == nil {
				h.log.Success(fmt.Sprintf(
					"remote template rendered for order %s (%d bytes, token=%s)",
					orderNum, len(pdfBytes), pdf.MaskToken(token),
				))
				return h.dispatchPDF(ctx, orderNum, pdfBytes, req.CustomerEmail)
			}
			if !h.cfg.PDF.AllowLocalFallback {
				h.log.Error(fmt.Sprintf(
					"remote template fetch failed for order %s (token=%s) — print/email skipped (AllowLocalFallback=false)",
					orderNum, pdf.MaskToken(token),
				), err)
				return nil
			}
			h.log.Warn(fmt.Sprintf(
				"remote template fetch failed for order %s (token=%s): %v — falling back to local template",
				orderNum, pdf.MaskToken(token), err,
			))
		}
	}

	// Local template path — unchanged behavior.
	pdfBytes, err := h.renderLocalInvoicePDF(ctx, req, result, orderNum)
	if err != nil {
		return err
	}
	h.log.Info(fmt.Sprintf("PDF generated locally for order %s (%d bytes)", orderNum, len(pdfBytes)))
	return h.dispatchPDF(ctx, orderNum, pdfBytes, req.CustomerEmail)
}

// fetchRemoteHTMLAndRenderPDF asks the backend for the rendered HTML, then runs
// the existing chromedp HTML→PDF pipeline. The token never appears in errors.
func (h *PDFPostOrderHook) fetchRemoteHTMLAndRenderPDF(ctx context.Context, token string, req OrderRequest) ([]byte, error) {
	timeout := time.Duration(h.cfg.PDF.RemoteTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	fetcher := pdf.NewRemoteFetcher(
		h.cfg.PDF.RemoteTemplateBaseURL,
		timeout,
		"erp-connector/"+connectorVersion,
	)

	fetchCtx, fetchCancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer fetchCancel()

	htmlBytes, err := fetcher.Fetch(
		fetchCtx,
		token,
		strings.ToLower(req.DocumentType),
		fmt.Sprintf("%s", req.HistoryID),
		req.UserExtID,
	)
	if err != nil {
		return nil, fmt.Errorf("remote fetch: %w", err)
	}

	pdfCtx, pdfCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pdfCancel()
	pdfBytes, err := h.pdfGen.GenerateFromHTML(pdfCtx, htmlBytes)
	if err != nil {
		return nil, fmt.Errorf("html→pdf: %w", err)
	}
	return pdfBytes, nil
}

// renderLocalInvoicePDF preserves the previous behavior — render the embedded
// invoice template using order data + local PDFConfig branding.
func (h *PDFPostOrderHook) renderLocalInvoicePDF(ctx context.Context, req OrderRequest, result *OrderResult, orderNum string) ([]byte, error) {
	var logoDataURI string
	if h.cfg.PDF.LogoPath == "" {
		h.log.Info("logo path not configured — PDF will have no logo")
	} else {
		if data, err := os.ReadFile(h.cfg.PDF.LogoPath); err == nil {
			mimeType := http.DetectContentType(data)
			logoDataURI = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
			h.log.Info(fmt.Sprintf("logo loaded: path=%s size=%d mime=%s", h.cfg.PDF.LogoPath, len(data), mimeType))
		} else {
			h.log.Warn(fmt.Sprintf("cannot read logo file: path=%s err=%v", h.cfg.PDF.LogoPath, err))
		}
	}

	invoiceData := pdf.InvoiceData{
		CompanyName:         h.cfg.PDF.CompanyName,
		CompanyAddress:      h.cfg.PDF.CompanyAddress,
		CompanyPhone:        h.cfg.PDF.CompanyPhone,
		CompanyFax:          h.cfg.PDF.CompanyFax,
		CompanyEmail:        h.cfg.PDF.CompanyEmail,
		LogoDataURI:         template.URL(logoDataURI),
		FooterHTML:          template.HTML(h.cfg.PDF.FooterHTML),
		DocumentNumber:      orderNum,
		Date:                formatDate(req.CreatedDate),
		CustomerName:        result.Account.FullName,
		CustomerPhone:       result.Account.Phone,
		CustomerCompany:     result.Account.AccountKey,
		Comment:             req.Comment,
		TotalBeforeDiscount: fmt.Sprintf("%.2f", req.Total),
		DiscountPercent:     fmt.Sprintf("%.2f", req.Discount),
		TotalAfterDiscount:  fmt.Sprintf("%.2f", req.Total*(1-req.Discount/100)),
		TaxPercent:          "17",
		TaxAmount:           fmt.Sprintf("%.2f", req.Total*(1-req.Discount/100)*0.17),
		TotalDue:            fmt.Sprintf("%.2f", req.Total*(1-req.Discount/100)*1.17),
	}

	for i, d := range req.Details {
		invoiceData.Items = append(invoiceData.Items, pdf.InvoiceItem{
			Index:       i + 1,
			SKU:         d.SKU,
			Title:       d.Title,
			Quantity:    fmt.Sprintf("%.2f", d.Quantity),
			UnitPrice:   fmt.Sprintf("%.2f", d.OriginalPrice),
			DiscountPct: fmt.Sprintf("%.2f", d.Discount),
			Total:       fmt.Sprintf("%.2f", d.TotalPrice),
		})
	}

	pdfCtx, pdfCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pdfCancel()

	pdfBytes, err := h.pdfGen.Generate(pdfCtx, invoiceData)
	if err != nil {
		return nil, fmt.Errorf("generate PDF for order %s: %w", orderNum, err)
	}
	return pdfBytes, nil
}

// dispatchPDF saves the PDF to the order's history dir and optionally prints
// and/or emails it. Behavior is identical to the previous inline path.
func (h *PDFPostOrderHook) dispatchPDF(ctx context.Context, orderNum string, pdfBytes []byte, customerEmail string) error {
	historyDir := filepath.Join(h.cfg.SendOrderDir, "history", orderNum)
	pdfPath := filepath.Join(historyDir, fmt.Sprintf("invoice_%s.pdf", orderNum))
	if err := os.MkdirAll(historyDir, 0o755); err == nil {
		if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
			h.log.Warn(fmt.Sprintf("failed to save PDF to history: %v", err))
		} else {
			h.log.Info(fmt.Sprintf("PDF saved to %s", pdfPath))
		}
	}

	if h.cfg.PDF.PrintAfterOrder {
		if err := print.PrintPDF(ctx, pdfPath, h.cfg.PDF.PrinterName, h.cfg.PDF.SumatraPDFPath); err != nil {
			h.log.Warn(fmt.Sprintf("print failed for order %s: %v", orderNum, err))
		} else {
			h.log.Success(fmt.Sprintf("PDF printed for order %s", orderNum))
		}
	}

	if h.cfg.PDF.EmailAfterOrder && h.emailSend != nil {
		if customerEmail == "" {
			h.log.Warn(fmt.Sprintf("email after order enabled but no customer email for order %s", orderNum))
		} else {
			if err := h.emailSend.SendInvoice(ctx, customerEmail, pdfBytes, orderNum); err != nil {
				h.log.Warn(fmt.Sprintf("email failed for order %s: %v", orderNum, err))
			} else {
				h.log.Success(fmt.Sprintf("PDF emailed to %s for order %s", customerEmail, orderNum))
			}
		}
	}

	return nil
}

// lookupRemoteToken returns the token for the given documentType, trying both
// the original case and lowercase form. Backend documentTypes are lowercase
// (e.g. "order"); the OrderRequest.DocumentType from Hasavshevet is uppercase
// (e.g. "ORDER") — so the operator may have stored the token under either.
func lookupRemoteToken(tokens map[string]string, documentType string) string {
	if len(tokens) == 0 || documentType == "" {
		return ""
	}
	if t, ok := tokens[documentType]; ok && t != "" {
		return t
	}
	if t, ok := tokens[strings.ToLower(documentType)]; ok && t != "" {
		return t
	}
	if t, ok := tokens[strings.ToUpper(documentType)]; ok && t != "" {
		return t
	}
	return ""
}

func formatDate(dateStr string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return fmt.Sprintf("%02d/%02d/%04d", t.Day(), int(t.Month()), t.Year())
		}
	}
	return dateStr
}
