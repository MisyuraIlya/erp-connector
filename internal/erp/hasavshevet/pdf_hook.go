package hasavshevet

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"erp-connector/internal/config"
	"erp-connector/internal/email"
	"erp-connector/internal/pdf"
	"erp-connector/internal/print"
	"erp-connector/internal/logger"
)

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
func (h *PDFPostOrderHook) AfterOrder(ctx context.Context, req OrderRequest, result *OrderResult) error {
	orderNum := fmt.Sprintf("%d", result.OrderNumber)

	// Load logo as base64 data URI if configured
	var logoDataURI string
	if h.cfg.PDF.LogoPath != "" {
		if data, err := os.ReadFile(h.cfg.PDF.LogoPath); err == nil {
			mimeType := http.DetectContentType(data)
			logoDataURI = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		} else {
			h.log.Warn(fmt.Sprintf("failed to read logo file %s: %v", h.cfg.PDF.LogoPath, err))
		}
	}

	// Map order data to PDF invoice data
	invoiceData := pdf.InvoiceData{
		CompanyName:    h.cfg.PDF.CompanyName,
		CompanyAddress: h.cfg.PDF.CompanyAddress,
		CompanyPhone:   h.cfg.PDF.CompanyPhone,
		CompanyFax:     h.cfg.PDF.CompanyFax,
		CompanyEmail:   h.cfg.PDF.CompanyEmail,
		LogoDataURI:    logoDataURI,
		FooterHTML:     template.HTML(h.cfg.PDF.FooterHTML),
		DocumentNumber: orderNum,
		Date:           formatDate(req.CreatedDate),
		CustomerName:   result.Account.FullName,
		CustomerPhone:  result.Account.Phone,
		CustomerCompany: result.Account.AccountKey,
		Comment:        req.Comment,
		TotalBeforeDiscount: fmt.Sprintf("%.2f", req.Total),
		DiscountPercent:     fmt.Sprintf("%.2f", req.Discount),
		TotalAfterDiscount:  fmt.Sprintf("%.2f", req.Total*(1-req.Discount/100)),
		TaxPercent:          "17",
		TaxAmount:           fmt.Sprintf("%.2f", req.Total*(1-req.Discount/100)*0.17),
		TotalDue:            fmt.Sprintf("%.2f", req.Total*(1-req.Discount/100)*1.17),
	}

	// Map line items
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

	// Generate PDF
	pdfCtx, pdfCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pdfCancel()

	pdfBytes, err := h.pdfGen.Generate(pdfCtx, invoiceData)
	if err != nil {
		return fmt.Errorf("generate PDF for order %s: %w", orderNum, err)
	}

	h.log.Info(fmt.Sprintf("PDF generated for order %s (%d bytes)", orderNum, len(pdfBytes)))

	// Save PDF to history directory
	historyDir := filepath.Join(h.cfg.SendOrderDir, "history", orderNum)
	pdfPath := filepath.Join(historyDir, fmt.Sprintf("invoice_%s.pdf", orderNum))
	if err := os.MkdirAll(historyDir, 0o755); err == nil {
		if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
			h.log.Warn(fmt.Sprintf("failed to save PDF to history: %v", err))
		} else {
			h.log.Info(fmt.Sprintf("PDF saved to %s", pdfPath))
		}
	}

	// Print if enabled
	if h.cfg.PDF.PrintAfterOrder {
		if err := print.PrintPDF(ctx, pdfPath, h.cfg.PDF.PrinterName, h.cfg.PDF.SumatraPDFPath); err != nil {
			h.log.Warn(fmt.Sprintf("print failed for order %s: %v", orderNum, err))
		} else {
			h.log.Success(fmt.Sprintf("PDF printed for order %s", orderNum))
		}
	}

	// Email if enabled
	if h.cfg.PDF.EmailAfterOrder && h.emailSend != nil {
		// Use customer email from order if available, otherwise skip
		recipientEmail := req.CustomerEmail
		if recipientEmail == "" {
			h.log.Warn(fmt.Sprintf("email after order enabled but no customer email for order %s", orderNum))
		} else {
			if err := h.emailSend.SendInvoice(ctx, recipientEmail, pdfBytes, orderNum); err != nil {
				h.log.Warn(fmt.Sprintf("email failed for order %s: %v", orderNum, err))
			} else {
				h.log.Success(fmt.Sprintf("PDF emailed to %s for order %s", recipientEmail, orderNum))
			}
		}
	}

	return nil
}

func formatDate(dateStr string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return fmt.Sprintf("%02d/%02d/%04d", t.Day(), int(t.Month()), t.Year())
		}
	}
	return dateStr
}
