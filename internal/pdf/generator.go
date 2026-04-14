package pdf

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/page"
)

// Generator produces PDF bytes from InvoiceData using headless Chrome.
type Generator struct {
	chromePath string
}

// NewGenerator creates a PDF generator. If chromePath is empty, chromedp
// will attempt to find Chrome automatically.
func NewGenerator(chromePath string) *Generator {
	return &Generator{chromePath: chromePath}
}

// Generate renders the invoice HTML template and converts it to PDF via headless Chrome.
func (g *Generator) Generate(ctx context.Context, data InvoiceData) ([]byte, error) {
	htmlStr, err := renderInvoiceHTML(data)
	if err != nil {
		return nil, fmt.Errorf("render html: %w", err)
	}

	dataURI := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(htmlStr))

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	if g.chromePath != "" {
		opts = append(opts, chromedp.ExecPath(g.chromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	var pdfBuf []byte
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(dataURI),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPaperWidth(8.27).   // A4 width in inches
				WithPaperHeight(11.69). // A4 height in inches
				WithMarginTop(0).       // margins handled by CSS @page
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithMarginRight(0).
				WithPrintBackground(true).
				Do(ctx)
			if err != nil {
				return err
			}
			pdfBuf = buf
			return nil
		}),
	); err != nil {
		return nil, fmt.Errorf("chromedp pdf generation: %w", err)
	}

	return pdfBuf, nil
}

// GenerateSample creates a sample invoice PDF with dummy data for testing.
func (g *Generator) GenerateSample(ctx context.Context, companyName, companyAddress, companyPhone, companyFax, companyEmail, logoPath, footerHTML string) ([]byte, error) {
	var logoDataURI string
	if logoPath != "" {
		if data, err := os.ReadFile(logoPath); err == nil {
			mimeType := http.DetectContentType(data)
			logoDataURI = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
	}

	data := InvoiceData{
		CompanyName:    companyName,
		CompanyAddress: companyAddress,
		CompanyPhone:   companyPhone,
		CompanyFax:     companyFax,
		CompanyEmail:   companyEmail,
		LogoDataURI:    logoDataURI,
		FooterHTML:     template.HTML(footerHTML),
		DocumentNumber: "12345",
		Date:           "13/04/2026",
		CustomerName:   "ישראל ישראלי",
		CustomerPhone:  "050-1234567",
		CustomerCompany: "חברה לדוגמה בע\"מ",
		Comment:        "הזמנת בדיקה",
		Items: []InvoiceItem{
			{Index: 1, SKU: "SKU-001", Title: "מוצר ראשון לבדיקה", Quantity: "10", UnitPrice: "50.00", DiscountPct: "5", Total: "475.00"},
			{Index: 2, SKU: "SKU-002", Title: "מוצר שני לבדיקה", Quantity: "5", UnitPrice: "100.00", DiscountPct: "0", Total: "500.00"},
			{Index: 3, SKU: "SKU-003", Title: "מוצר שלישי לבדיקה", Quantity: "2", UnitPrice: "250.00", DiscountPct: "10", Total: "450.00"},
		},
		TotalBeforeDiscount: "1500.00",
		DiscountPercent:     "5.00",
		TotalAfterDiscount:  "1425.00",
		TaxPercent:          "17",
		TaxAmount:           "242.25",
		TotalDue:            "1667.25",
	}

	return g.Generate(ctx, data)
}
