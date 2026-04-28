//go:build windows

package main

import (
	"context"
	"fmt"
	"net/http"
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

	// Remote template fields (mission 022)
	var remoteBaseURLEdit, remoteTimeoutEdit, remoteTestDocTypeEdit, remoteTestDocNumberEdit, remoteTestUserExtIDEdit *walk.LineEdit
	var remoteTokensEdit *walk.TextEdit
	var useRemoteTemplateCheck, allowLocalFallbackCheck *walk.CheckBox

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

							// ── Logo diagnostic (shown in status bar + log) ──────────
							logoPath := strings.TrimSpace(logoPathEdit.Text())
							var logoDiag string
							if logoPath == "" {
								logoDiag = "logo: not set"
								logSvc.Info("test-print: logo path is empty")
							} else {
								if logoData, readErr := os.ReadFile(logoPath); readErr != nil {
									logoDiag = "logo ERROR: " + readErr.Error()
									logSvc.Warn("test-print: logo read error: " + readErr.Error())
								} else {
									mimeType := http.DetectContentType(logoData)
									logoDiag = fmt.Sprintf("logo OK: %s (%d B)", mimeType, len(logoData))
									logSvc.Info(fmt.Sprintf("test-print: logo OK path=%s size=%d mime=%s", logoPath, len(logoData), mimeType))
								}
							}
							dlg.Synchronize(func() { setStatus(logoDiag + " | generating PDF...") })

							ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
							defer cancel()
							gen := pdf.NewGenerator(chromePath)
							pdfBytes, err := gen.GenerateSample(ctx,
								strings.TrimSpace(companyNameEdit.Text()),
								strings.TrimSpace(companyAddressEdit.Text()),
								strings.TrimSpace(companyPhoneEdit.Text()),
								strings.TrimSpace(companyFaxEdit.Text()),
								strings.TrimSpace(companyEmailEdit.Text()),
								logoPath,
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
									setStatus("Print failed: " + printErr.Error() + " | " + logoDiag + saved)
								} else {
									setStatus("Print OK | " + logoDiag + saved)
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

					// ── Remote Template (mission 022) ────────────────
					HSeparator{},
					Label{Text: "Remote PDF Template", Font: Font{Bold: true}},
					Label{Text: "Backend Base URL (e.g. https://api.example.com — no /api suffix)"},
					LineEdit{AssignTo: &remoteBaseURLEdit, CueBanner: "https://api.example.com"},
					CheckBox{AssignTo: &useRemoteTemplateCheck, Text: "Use remote template (admin-customized PDF)"},
					CheckBox{AssignTo: &allowLocalFallbackCheck, Text: "Fall back to local template if remote fetch fails"},
					Label{Text: "Timeout (seconds, default 15)"},
					LineEdit{AssignTo: &remoteTimeoutEdit, CueBanner: "15"},
					Label{Text: "Tokens (one per line: documentType=token)"},
					TextEdit{AssignTo: &remoteTokensEdit, MinSize: Size{Height: 80}, VScroll: true},
					Label{Text: "Test fetch — documentType / documentNumber / userExtId"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							LineEdit{AssignTo: &remoteTestDocTypeEdit, CueBanner: "order"},
							LineEdit{AssignTo: &remoteTestDocNumberEdit, CueBanner: "ORD-1"},
							LineEdit{AssignTo: &remoteTestUserExtIDEdit, CueBanner: "USR-9"},
							PushButton{Text: "Test fetch", OnClicked: func() {
								setStatus("Testing remote fetch...")
								go func() {
									baseURL := strings.TrimSpace(remoteBaseURLEdit.Text())
									tokens := parseRemoteTokens(remoteTokensEdit.Text())
									docType := strings.TrimSpace(remoteTestDocTypeEdit.Text())
									if docType == "" {
										docType = "order"
									}
									token := tokens[docType]
									if token == "" {
										dlg.Synchronize(func() { setStatus("No token configured for documentType=" + docType) })
										return
									}
									timeoutSecs, _ := strconv.Atoi(remoteTimeoutEdit.Text())
									if timeoutSecs <= 0 {
										timeoutSecs = 15
									}
									fetcher := pdf.NewRemoteFetcher(baseURL, time.Duration(timeoutSecs)*time.Second, "erp-connector/test")
									ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs+5)*time.Second)
									defer cancel()
									body, err := fetcher.Fetch(ctx,
										token, docType,
										strings.TrimSpace(remoteTestDocNumberEdit.Text()),
										strings.TrimSpace(remoteTestUserExtIDEdit.Text()),
									)
									dlg.Synchronize(func() {
										if err != nil {
											setStatus("Test fetch failed (token=" + pdf.MaskToken(token) + "): " + err.Error())
											return
										}
										preview := string(body)
										if len(preview) > 200 {
											preview = preview[:200] + "..."
										}
										setStatus(fmt.Sprintf("Test fetch OK (%d bytes, token=%s) — preview: %s", len(body), pdf.MaskToken(token), preview))
									})
								}()
							}},
						},
					},

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

						cfg.PDF.RemoteTemplateBaseURL = strings.TrimSpace(remoteBaseURLEdit.Text())
						cfg.PDF.UseRemoteTemplate = useRemoteTemplateCheck.Checked()
						cfg.PDF.AllowLocalFallback = allowLocalFallbackCheck.Checked()
						remoteTimeoutSecs, _ := strconv.Atoi(strings.TrimSpace(remoteTimeoutEdit.Text()))
						if remoteTimeoutSecs > 0 {
							cfg.PDF.RemoteTimeoutSeconds = remoteTimeoutSecs
						}
						cfg.PDF.RemoteTokens = parseRemoteTokens(remoteTokensEdit.Text())

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

	remoteBaseURLEdit.SetText(cfg.PDF.RemoteTemplateBaseURL)
	useRemoteTemplateCheck.SetChecked(cfg.PDF.UseRemoteTemplate)
	allowLocalFallbackCheck.SetChecked(cfg.PDF.AllowLocalFallback)
	if cfg.PDF.RemoteTimeoutSeconds > 0 {
		remoteTimeoutEdit.SetText(strconv.Itoa(cfg.PDF.RemoteTimeoutSeconds))
	}
	remoteTokensEdit.SetText(formatRemoteTokens(cfg.PDF.RemoteTokens))

	dlg.Run()
}

// parseRemoteTokens parses a multi-line "documentType=token" textarea into a
// map. Whitespace is trimmed; comment lines (# or //) and blank lines are
// skipped; duplicate documentTypes — last entry wins.
func parseRemoteTokens(text string) map[string]string {
	out := map[string]string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// formatRemoteTokens renders the in-memory map back to "documentType=token"
// lines for display in the textarea.
func formatRemoteTokens(tokens map[string]string) string {
	if len(tokens) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tokens))
	for k := range tokens {
		keys = append(keys, k)
	}
	// stable order without importing sort: keys already random in map iteration,
	// so do a tiny insertion sort to keep the output deterministic across saves.
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && keys[j-1] > keys[j] {
			keys[j-1], keys[j] = keys[j], keys[j-1]
			j--
		}
	}
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(tokens[k])
		b.WriteByte('\n')
	}
	return b.String()
}
