//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"erp-connector/internal/config"
	"erp-connector/internal/email"
	"erp-connector/internal/logger"
	"erp-connector/internal/pdf"
	pdfprint "erp-connector/internal/print"
	"erp-connector/internal/platform/paths"
	"erp-connector/internal/secrets"
)

// showPDFSettingsDialog opens a separate window for PDF, Print, and Email configuration.
// Changes are saved directly to config when the user clicks Save.
func showPDFSettingsDialog(owner walk.Form, cfg *config.Config, logSvc logger.LoggerService) {
	var dlg *walk.Dialog
	var statusLabel *walk.Label

	// PDF fields
	var companyNameEdit, companyAddressEdit, companyPhoneEdit *walk.LineEdit
	var companyFaxEdit, companyEmailEdit, logoPathEdit, footerEdit *walk.LineEdit
	var chromePathEdit, sumatraPathEdit, printerNameEdit *walk.LineEdit
	var printAfterOrderCheck *walk.CheckBox

	// Email fields
	var emailAfterOrderCheck *walk.CheckBox
	var smtpHostEdit, smtpPortEdit, smtpUserEdit, smtpPassEdit *walk.LineEdit
	var smtpFromEdit *walk.LineEdit
	var smtpTLSCheck *walk.CheckBox

	setStatus := func(text string) {
		if statusLabel != nil {
			statusLabel.SetText(text)
		}
	}

	err := (Dialog{
		AssignTo: &dlg,
		Title:    "PDF & Email Settings",
		MinSize:  Size{Width: 500, Height: 600},
		Size:     Size{Width: 520, Height: 700},
		Layout:   VBox{},
		Children: []Widget{
			ScrollView{
				Layout: VBox{},
				Children: []Widget{
					// ── Company Branding ──────────────────────────────
					Label{Text: "Company Branding", Font: Font{Bold: true}},
					Label{Text: "Company Name"},
					LineEdit{AssignTo: &companyNameEdit},
					Label{Text: "Company Address"},
					LineEdit{AssignTo: &companyAddressEdit},
					Label{Text: "Company Phone"},
					LineEdit{AssignTo: &companyPhoneEdit},
					Label{Text: "Company Fax"},
					LineEdit{AssignTo: &companyFaxEdit},
					Label{Text: "Company Email"},
					LineEdit{AssignTo: &companyEmailEdit},
					Label{Text: "Logo File"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							LineEdit{AssignTo: &logoPathEdit},
							PushButton{Text: "Browse...", OnClicked: func() {
								fd := &walk.FileDialog{
									Title:  "Select logo image",
									Filter: "Image Files (*.png;*.jpg;*.jpeg;*.bmp)|*.png;*.jpg;*.jpeg;*.bmp|All Files (*.*)|*.*",
								}
								if ok, err := fd.ShowOpen(dlg); err != nil {
									setStatus("File selection error: " + err.Error())
								} else if ok {
									logoPathEdit.SetText(fd.FilePath)
								}
							}},
						},
					},
					Label{Text: "Footer Text (HTML)"},
					LineEdit{AssignTo: &footerEdit},

					// ── PDF Engine ───────────────────────────────────
					HSeparator{},
					Label{Text: "PDF Engine", Font: Font{Bold: true}},
					Label{Text: "Chrome Path"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							LineEdit{AssignTo: &chromePathEdit, CueBanner: "Auto-detected if empty"},
							PushButton{Text: "Browse...", OnClicked: func() {
								fd := &walk.FileDialog{
									Title:  "Select Chrome/Chromium",
									Filter: "Executable (*.exe)|*.exe|All Files (*.*)|*.*",
								}
								if ok, err := fd.ShowOpen(dlg); err != nil {
									setStatus("Error: " + err.Error())
								} else if ok {
									chromePathEdit.SetText(fd.FilePath)
								}
							}},
							PushButton{Text: "Auto-detect", OnClicked: func() {
								p := pdf.DetectChrome()
								if p == "" {
									setStatus("Chrome not found on this system")
								} else {
									chromePathEdit.SetText(p)
									setStatus("Chrome found: " + p)
								}
							}},
						},
					},
					Label{Text: "SumatraPDF Path"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							LineEdit{AssignTo: &sumatraPathEdit, CueBanner: "Auto-detected if empty"},
							PushButton{Text: "Browse...", OnClicked: func() {
								fd := &walk.FileDialog{
									Title:  "Select SumatraPDF",
									Filter: "Executable (*.exe)|*.exe|All Files (*.*)|*.*",
								}
								if ok, err := fd.ShowOpen(dlg); err != nil {
									setStatus("Error: " + err.Error())
								} else if ok {
									sumatraPathEdit.SetText(fd.FilePath)
								}
							}},
						},
					},

					// ── Print Settings ───────────────────────────────
					HSeparator{},
					Label{Text: "Print Settings", Font: Font{Bold: true}},
					CheckBox{AssignTo: &printAfterOrderCheck, Text: "Print PDF after send order"},
					Label{Text: "Printer Name (empty = default)"},
					LineEdit{AssignTo: &printerNameEdit, CueBanner: "Leave empty for default printer"},
					PushButton{Text: "Test Print", OnClicked: func() {
						setStatus("Generating test PDF and printing...")
						go func() {
							chromePath := strings.TrimSpace(chromePathEdit.Text())
							if chromePath == "" {
								chromePath = pdf.DetectChrome()
							}
							if chromePath == "" {
								dlg.Synchronize(func() { setStatus("Chrome not found; cannot generate PDF") })
								return
							}
							ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
							defer cancel()
							gen := pdf.NewGenerator(chromePath)
							pdfBytes, err := gen.GenerateSample(ctx,
								strings.TrimSpace(companyNameEdit.Text()),
								strings.TrimSpace(companyAddressEdit.Text()),
								strings.TrimSpace(companyPhoneEdit.Text()),
								strings.TrimSpace(companyFaxEdit.Text()),
								strings.TrimSpace(companyEmailEdit.Text()),
								strings.TrimSpace(logoPathEdit.Text()),
								strings.TrimSpace(footerEdit.Text()),
							)
							if err != nil {
								dlg.Synchronize(func() { setStatus("PDF generation failed: " + err.Error()) })
								return
							}

							// Save a copy to ProgramData so the PDF can be inspected during development.
							var savedPath string
							dataDir := paths.DataDir()
							if mkErr := os.MkdirAll(dataDir, 0o755); mkErr == nil {
								saveName := "test_print_" + time.Now().Format("20060102_150405") + ".pdf"
								candidate := filepath.Join(dataDir, saveName)
								if writeErr := os.WriteFile(candidate, pdfBytes, 0o644); writeErr == nil {
									savedPath = candidate
								}
							}

							tmpFile := filepath.Join(os.TempDir(), "erp_connector_test_print.pdf")
							if err := os.WriteFile(tmpFile, pdfBytes, 0o644); err != nil {
								dlg.Synchronize(func() { setStatus("Failed to write temp PDF: " + err.Error()) })
								return
							}
							defer os.Remove(tmpFile)
							printErr := pdfprint.PrintPDF(ctx, tmpFile,
								strings.TrimSpace(printerNameEdit.Text()),
								strings.TrimSpace(sumatraPathEdit.Text()))
							dlg.Synchronize(func() {
								saved := ""
								if savedPath != "" {
									saved = " | Saved: " + savedPath
								}
								if printErr != nil {
									setStatus("Print failed: " + printErr.Error() + saved)
								} else {
									setStatus("Test print sent to printer successfully." + saved)
								}
							})
						}()
					}},

					// ── Email Settings ───────────────────────────────
					HSeparator{},
					Label{Text: "Email Settings", Font: Font{Bold: true}},
					CheckBox{AssignTo: &emailAfterOrderCheck, Text: "Send PDF by email after send order"},
					Label{Text: "SMTP Host"},
					LineEdit{AssignTo: &smtpHostEdit},
					Label{Text: "SMTP Port"},
					LineEdit{AssignTo: &smtpPortEdit, CueBanner: "587"},
					Label{Text: "SMTP User"},
					LineEdit{AssignTo: &smtpUserEdit},
					Label{Text: "SMTP Password"},
					LineEdit{AssignTo: &smtpPassEdit, PasswordMode: true, CueBanner: "Leave blank to keep existing"},
					Label{Text: "From Address"},
					LineEdit{AssignTo: &smtpFromEdit},
					CheckBox{AssignTo: &smtpTLSCheck, Text: "Use TLS"},
					PushButton{Text: "Test Email", OnClicked: func() {
						setStatus("Sending test email...")
						go func() {
							chromePath := strings.TrimSpace(chromePathEdit.Text())
							if chromePath == "" {
								chromePath = pdf.DetectChrome()
							}
							var pdfBytes []byte
							if chromePath != "" {
								ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
								defer cancel()
								gen := pdf.NewGenerator(chromePath)
								var err error
								pdfBytes, err = gen.GenerateSample(ctx,
									strings.TrimSpace(companyNameEdit.Text()),
									strings.TrimSpace(companyAddressEdit.Text()),
									strings.TrimSpace(companyPhoneEdit.Text()),
									strings.TrimSpace(companyFaxEdit.Text()),
									strings.TrimSpace(companyEmailEdit.Text()),
									strings.TrimSpace(logoPathEdit.Text()),
									strings.TrimSpace(footerEdit.Text()),
								)
								if err != nil {
									dlg.Synchronize(func() { setStatus("PDF generation failed: " + err.Error()) })
									return
								}
							}
							smtpPass := smtpPassEdit.Text()
							if smtpPass == "" {
								if stored, err := secrets.Get("smtp_password"); err == nil {
									smtpPass = string(stored)
								}
							}
							smtpPort, _ := strconv.Atoi(smtpPortEdit.Text())
							if smtpPort <= 0 || smtpPort > 65535 {
								smtpPort = 587
							}
							smtpCfg := config.SMTPConfig{
								Host:        strings.TrimSpace(smtpHostEdit.Text()),
								Port:        smtpPort,
								User:        strings.TrimSpace(smtpUserEdit.Text()),
								FromAddress: strings.TrimSpace(smtpFromEdit.Text()),
								UseTLS:      smtpTLSCheck.Checked(),
							}
							sender := email.NewSender(smtpCfg, smtpPass)
							ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
							defer cancel()
							err := sender.SendTestEmail(ctx, pdfBytes)
							dlg.Synchronize(func() {
								if err != nil {
									setStatus("Email test failed: " + err.Error())
								} else {
									setStatus("Test email sent to " + smtpCfg.FromAddress)
								}
							})
						}()
					}},

					// ── Status + Save ────────────────────────────────
					HSeparator{},
					Label{AssignTo: &statusLabel},
				},
			},
			// Save and Close buttons at the bottom, outside ScrollView
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "Save", OnClicked: func() {
						// Read all fields into config
						cfg.PDF.CompanyName = strings.TrimSpace(companyNameEdit.Text())
						cfg.PDF.CompanyAddress = strings.TrimSpace(companyAddressEdit.Text())
						cfg.PDF.CompanyPhone = strings.TrimSpace(companyPhoneEdit.Text())
						cfg.PDF.CompanyFax = strings.TrimSpace(companyFaxEdit.Text())
						cfg.PDF.CompanyEmail = strings.TrimSpace(companyEmailEdit.Text())
						cfg.PDF.LogoPath = strings.TrimSpace(logoPathEdit.Text())
						cfg.PDF.FooterHTML = strings.TrimSpace(footerEdit.Text())
						cfg.PDF.ChromePath = strings.TrimSpace(chromePathEdit.Text())
						cfg.PDF.SumatraPDFPath = strings.TrimSpace(sumatraPathEdit.Text())
						cfg.PDF.PrintAfterOrder = printAfterOrderCheck.Checked()
						cfg.PDF.PrinterName = strings.TrimSpace(printerNameEdit.Text())
						cfg.PDF.EmailAfterOrder = emailAfterOrderCheck.Checked()
						cfg.SMTP.Host = strings.TrimSpace(smtpHostEdit.Text())
						smtpPort, _ := strconv.Atoi(smtpPortEdit.Text())
						if smtpPort <= 0 || smtpPort > 65535 {
							smtpPort = 587
						}
						cfg.SMTP.Port = smtpPort
						cfg.SMTP.User = strings.TrimSpace(smtpUserEdit.Text())
						cfg.SMTP.FromAddress = strings.TrimSpace(smtpFromEdit.Text())
						cfg.SMTP.UseTLS = smtpTLSCheck.Checked()

						setStatus("Saving...")
						go func() {
							// Save SMTP password
							smtpPass := smtpPassEdit.Text()
							if smtpPass != "" {
								if err := secrets.Set("smtp_password", []byte(smtpPass)); err != nil {
									dlg.Synchronize(func() { setStatus("Failed to save SMTP password: " + err.Error()) })
									return
								}
							}
							// Save config
							if err := config.Save(*cfg); err != nil {
								dlg.Synchronize(func() { setStatus("Failed to save config: " + err.Error()) })
								return
							}
							dlg.Synchronize(func() { setStatus("נשמר בהצלחה.") })
						}()
					}},
					PushButton{Text: "Close", OnClicked: func() {
						dlg.Accept()
					}},
				},
			},
		},
	}).Create(owner)

	if err != nil {
		walk.MsgBox(owner, "Error", "Failed to create PDF settings window: "+err.Error(), walk.MsgBoxIconError)
		return
	}

	// Populate fields from current config
	companyNameEdit.SetText(cfg.PDF.CompanyName)
	companyAddressEdit.SetText(cfg.PDF.CompanyAddress)
	companyPhoneEdit.SetText(cfg.PDF.CompanyPhone)
	companyFaxEdit.SetText(cfg.PDF.CompanyFax)
	companyEmailEdit.SetText(cfg.PDF.CompanyEmail)
	logoPathEdit.SetText(cfg.PDF.LogoPath)
	footerEdit.SetText(cfg.PDF.FooterHTML)
	chromePathEdit.SetText(cfg.PDF.ChromePath)
	sumatraPathEdit.SetText(cfg.PDF.SumatraPDFPath)
	printAfterOrderCheck.SetChecked(cfg.PDF.PrintAfterOrder)
	printerNameEdit.SetText(cfg.PDF.PrinterName)
	emailAfterOrderCheck.SetChecked(cfg.PDF.EmailAfterOrder)
	smtpHostEdit.SetText(cfg.SMTP.Host)
	smtpPortEdit.SetText(strconv.Itoa(cfg.SMTP.Port))
	smtpUserEdit.SetText(cfg.SMTP.User)
	smtpFromEdit.SetText(cfg.SMTP.FromAddress)
	smtpTLSCheck.SetChecked(cfg.SMTP.UseTLS)

	dlg.Run()
}
