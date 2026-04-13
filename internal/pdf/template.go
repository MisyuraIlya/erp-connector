package pdf

import (
	"bytes"
	"fmt"
	"html/template"
)

// InvoiceData holds all data needed to render an invoice PDF.
type InvoiceData struct {
	// Company branding
	CompanyName    string
	CompanyAddress string
	CompanyPhone   string
	CompanyFax     string
	CompanyEmail   string
	LogoDataURI    string // base64 data URI or empty
	FooterHTML     template.HTML

	// Font
	FontDataURI string

	// Document
	DocumentNumber string
	Date           string
	CustomerName   string
	CustomerPhone  string
	CustomerCompany string
	Comment        string

	// Line items
	Items []InvoiceItem

	// Totals
	TotalBeforeDiscount string
	DiscountPercent     string
	TotalAfterDiscount  string
	TaxPercent          string
	TaxAmount           string
	TotalDue            string
}

// InvoiceItem represents one line item in the invoice table.
type InvoiceItem struct {
	Index       int
	SKU         string
	Title       string
	Quantity    string
	UnitPrice   string
	DiscountPct string
	Total       string
}

// renderInvoiceHTML executes the invoice HTML template with the provided data.
func renderInvoiceHTML(data InvoiceData) (string, error) {
	tmpl, err := template.New("invoice").Parse(invoiceTemplateHTML)
	if err != nil {
		return "", fmt.Errorf("parse invoice template: %w", err)
	}

	data.FontDataURI = fontDataURI()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute invoice template: %w", err)
	}
	return buf.String(), nil
}
