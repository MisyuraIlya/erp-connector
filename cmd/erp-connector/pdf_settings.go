//go:build windows

package main

import (
	"context"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"erp-connector/internal/config"
	"erp-connector/internal/email"
	"erp-connector/internal/logger"
	"erp-connector/internal/pdf"
	"erp-connector/internal/print"
	"erp-connector/internal/secrets"
)

const wsdSuffix = " (per-user)"

// formatPrinterLabel returns the dropdown label for a printer. WSD-port
// printers get a " (per-user)" suffix so operators see at-a-glance that the
// daemon's LocalSystem account cannot reach them. The bare Name is what gets
// persisted to cfg.PDF.PrinterName.
func formatPrinterLabel(p print.PrinterInfo) string {
	if p.IsWSD() {
		return p.Name + wsdSuffix
	}
	return p.Name
}

// stripPrinterLabelSuffix returns the bare printer name from a label that may
// include the " (per-user)" annotation. Safe to call on hand-typed text too.
func stripPrinterLabelSuffix(label string) string {
	label = strings.TrimSpace(label)
	if strings.HasSuffix(label, wsdSuffix) {
		return strings.TrimSuffix(label, wsdSuffix)
	}
	return label
}

// findPrinterByLabel returns the PrinterInfo whose formatted label matches
// `label`, or nil if no match. Used to drive the WSD-warning surface based on
// the current ComboBox selection.
func findPrinterByLabel(printers []print.PrinterInfo, label string) *print.PrinterInfo {
	bare := stripPrinterLabelSuffix(label)
	for i := range printers {
		if printers[i].Name == bare {
			return &printers[i]
		}
	}
	return nil
}

// showPDFSettingsDialog opens a separate window for PDF, Print, and Email configuration.
// Changes are saved directly to config when the user clicks Save.
func showPDFSettingsDialog(owner walk.Form, cfg *config.Config, logSvc logger.LoggerService) {
	var dlg *walk.Dialog
	var statusLabel *walk.Label

	// PDF engine fields
	var chromePathEdit, sumatraPathEdit *walk.LineEdit
	var printerCombo *walk.ComboBox
	var printerWarningLabel *walk.Label
	var printAfterOrderCheck *walk.CheckBox

	// detectedPrinters is the most recent ListPrinters() result. The ComboBox
	// model is derived from it so we can map a selected label back to its
	// PrinterInfo (port, driver, status) for warning logic.
	var detectedPrinters []print.PrinterInfo

	// Email fields
	var emailAfterOrderCheck *walk.CheckBox
	var smtpHostEdit, smtpPortEdit, smtpUserEdit, smtpPassEdit *walk.LineEdit
	var smtpFromEdit *walk.LineEdit
	var smtpTLSCheck *walk.CheckBox

	// Remote template fields
	var remoteBaseURLEdit, remoteTimeoutEdit, remoteTestDocTypeEdit, remoteTestDocNumberEdit, remoteTestUserExtIDEdit *walk.LineEdit
	var remoteTokensEdit *walk.TextEdit
	var useRemoteTemplateCheck *walk.CheckBox

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
					// Branding (company name, logo, footer) lives in the
					// backend AppSettings now — admin configures it there.

					// ── PDF Engine ───────────────────────────────────
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
					Label{Text: "Printer (empty = system default)"},
					ComboBox{
						AssignTo: &printerCombo,
						Editable: true,
						OnEditingFinished: func() {
							refreshPrinterWarning(printerCombo, printerWarningLabel, &detectedPrinters)
						},
						OnCurrentIndexChanged: func() {
							refreshPrinterWarning(printerCombo, printerWarningLabel, &detectedPrinters)
						},
					},
					Label{AssignTo: &printerWarningLabel, TextColor: walk.RGB(180, 90, 0)},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							PushButton{Text: "Refresh printers", OnClicked: func() {
								setStatus("Loading printers…")
								go func() {
									ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
									defer cancel()
									printers, err := print.ListPrinters(ctx)
									dlg.Synchronize(func() {
										if err != nil {
											setStatus("Refresh printers failed: " + err.Error())
											return
										}
										detectedPrinters = printers
										current := strings.TrimSpace(printerCombo.Text())
										labels := make([]string, 0, len(printers))
										for _, p := range printers {
											labels = append(labels, formatPrinterLabel(p))
										}
										_ = printerCombo.SetModel(labels)
										if current != "" {
											printerCombo.SetText(current)
										}
										refreshPrinterWarning(printerCombo, printerWarningLabel, &detectedPrinters)
										setStatus(fmt.Sprintf("Loaded %d printers.", len(printers)))
									})
								}()
							}},
							PushButton{Text: "Install network printer…", OnClicked: func() {
								showInstallPrinterDialog(dlg, func(installedName string) {
									// Refresh list after install + select new printer.
									go func() {
										ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
										defer cancel()
										printers, err := print.ListPrinters(ctx)
										dlg.Synchronize(func() {
											if err != nil {
												setStatus("Refresh after install failed: " + err.Error())
												return
											}
											detectedPrinters = printers
											labels := make([]string, 0, len(printers))
											for _, p := range printers {
												labels = append(labels, formatPrinterLabel(p))
											}
											_ = printerCombo.SetModel(labels)
											printerCombo.SetText(installedName)
											refreshPrinterWarning(printerCombo, printerWarningLabel, &detectedPrinters)
											setStatus("Installed printer '" + installedName + "'.")
										})
									}()
								})
							}},
							PushButton{Text: "Test print", OnClicked: func() {
								selected := stripPrinterLabelSuffix(printerCombo.Text())
								setStatus("Test print: rendering…")
								go runTestPrint(dlg, setStatus, selected,
									strings.TrimSpace(chromePathEdit.Text()),
									strings.TrimSpace(sumatraPathEdit.Text()),
									logSvc,
								)
							}},
						},
					},

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
							// Plain SMTP smoke test — no PDF attachment now that the
							// connector no longer carries a local invoice template.
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
							err := sender.SendTestEmail(ctx, nil)
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
					Label{Text: "Timeout (seconds, default 15)"},
					LineEdit{AssignTo: &remoteTimeoutEdit, CueBanner: "15"},
					Label{Text: "Tokens — one per line, format: documentType=token (e.g. order=abc123…). Paste only the token (no key) to use it for ALL document types."},
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
									rawTokens := remoteTokensEdit.Text()
									tokens := parseRemoteTokens(rawTokens)
									docType := strings.TrimSpace(remoteTestDocTypeEdit.Text())
									if docType == "" {
										docType = "order"
									}
									token := lookupTokenCaseInsensitive(tokens, docType)
									if token == "" {
										// Tolerant fallback: if the textarea contains a single
										// non-empty line with no `=`, treat it as the token for
										// any document type. Lets the operator paste only the
										// token without remembering the `documentType=` prefix.
										token = inferBareToken(rawTokens)
									}
									if token == "" {
										parsedKeys := make([]string, 0, len(tokens))
										for k := range tokens {
											parsedKeys = append(parsedKeys, k)
										}
										msg := fmt.Sprintf(
											"No token for documentType=%s. Parsed %d entries (keys: %v). Format must be `documentType=token` per line, or paste only the token.",
											docType, len(tokens), parsedKeys,
										)
										dlg.Synchronize(func() { setStatus(msg) })
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
						cfg.PDF.ChromePath = strings.TrimSpace(chromePathEdit.Text())
						cfg.PDF.SumatraPDFPath = strings.TrimSpace(sumatraPathEdit.Text())
						cfg.PDF.PrintAfterOrder = printAfterOrderCheck.Checked()
						cfg.PDF.PrinterName = stripPrinterLabelSuffix(printerCombo.Text())
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
									if logSvc != nil {
										logSvc.Error("PDF settings: failed to save SMTP password", err)
									}
									dlg.Synchronize(func() { setStatus("Failed to save SMTP password: " + err.Error()) })
									return
								}
							}
							// Save config
							if err := config.Save(*cfg); err != nil {
								if logSvc != nil {
									logSvc.Error("PDF settings: failed to save config", err)
								}
								dlg.Synchronize(func() { setStatus("Failed to save config: " + err.Error()) })
								return
							}
							if logSvc != nil {
								logSvc.Info(fmt.Sprintf(
									"PDF settings saved: PrintAfterOrder=%v EmailAfterOrder=%v UseRemoteTemplate=%v RemoteTemplateBaseURL=%q tokenCount=%d ChromePath=%q SumatraPDFPath=%q PrinterName=%q — RESTART erp-connectord daemon for changes to take effect",
									cfg.PDF.PrintAfterOrder, cfg.PDF.EmailAfterOrder, cfg.PDF.UseRemoteTemplate,
									cfg.PDF.RemoteTemplateBaseURL, len(cfg.PDF.RemoteTokens),
									cfg.PDF.ChromePath, cfg.PDF.SumatraPDFPath, cfg.PDF.PrinterName,
								))
							}
							dlg.Synchronize(func() { setStatus("נשמר בהצלחה. הפעל מחדש את erp-connectord (restart the daemon) כדי שהשינויים ייכנסו לתוקף.") })
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
	chromePathEdit.SetText(cfg.PDF.ChromePath)
	sumatraPathEdit.SetText(cfg.PDF.SumatraPDFPath)
	printAfterOrderCheck.SetChecked(cfg.PDF.PrintAfterOrder)
	// Pre-fill the typed value so the operator sees what's currently saved
	// even before the async ListPrinters() finishes.
	printerCombo.SetText(cfg.PDF.PrinterName)
	// Kick off async printer enumeration; populates the dropdown model.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		printers, err := print.ListPrinters(ctx)
		dlg.Synchronize(func() {
			if err != nil {
				setStatus("Could not list printers: " + err.Error())
				return
			}
			detectedPrinters = printers
			labels := make([]string, 0, len(printers))
			for _, p := range printers {
				labels = append(labels, formatPrinterLabel(p))
			}
			_ = printerCombo.SetModel(labels)
			// Re-set the text after model swap (Walk clears it on SetModel).
			if cfg.PDF.PrinterName != "" {
				for _, p := range printers {
					if p.Name == cfg.PDF.PrinterName {
						printerCombo.SetText(formatPrinterLabel(p))
						break
					}
				}
				if printerCombo.Text() == "" {
					printerCombo.SetText(cfg.PDF.PrinterName)
				}
			}
			refreshPrinterWarning(printerCombo, printerWarningLabel, &detectedPrinters)
		})
	}()
	emailAfterOrderCheck.SetChecked(cfg.PDF.EmailAfterOrder)
	smtpHostEdit.SetText(cfg.SMTP.Host)
	smtpPortEdit.SetText(strconv.Itoa(cfg.SMTP.Port))
	smtpUserEdit.SetText(cfg.SMTP.User)
	smtpFromEdit.SetText(cfg.SMTP.FromAddress)
	smtpTLSCheck.SetChecked(cfg.SMTP.UseTLS)

	remoteBaseURLEdit.SetText(cfg.PDF.RemoteTemplateBaseURL)
	useRemoteTemplateCheck.SetChecked(cfg.PDF.UseRemoteTemplate)
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

// lookupTokenCaseInsensitive returns the token for documentType, trying the
// raw key, lowercase, and uppercase forms — matches the runtime hook's
// lookupRemoteToken behavior so Test fetch and AfterOrder agree.
func lookupTokenCaseInsensitive(tokens map[string]string, documentType string) string {
	if len(tokens) == 0 || documentType == "" {
		return ""
	}
	if t, ok := tokens[documentType]; ok && t != "" {
		return t
	}
	lower := strings.ToLower(documentType)
	upper := strings.ToUpper(documentType)
	if t, ok := tokens[lower]; ok && t != "" {
		return t
	}
	if t, ok := tokens[upper]; ok && t != "" {
		return t
	}
	for k, v := range tokens {
		if strings.EqualFold(k, documentType) && v != "" {
			return v
		}
	}
	return ""
}

// inferBareToken returns the only non-comment, non-empty line in `text` if
// that line contains no `=`. This lets operators paste a single token without
// remembering the `documentType=` key. Returns "" when the textarea has zero
// or multiple bare lines, or when any line already has a key=value form.
func inferBareToken(text string) string {
	var bare string
	count := 0
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.ContainsRune(line, '=') {
			return "" // structured input present — do not guess
		}
		bare = line
		count++
	}
	if count == 1 {
		return bare
	}
	return ""
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

// refreshPrinterWarning updates the small per-user warning label below the
// printer ComboBox based on the currently selected/typed printer. WSD-port
// printers cannot be reached by the LocalSystem-running daemon, so we surface
// that as a non-blocking warning rather than a hard error — the operator may
// still want to keep the field empty (system default) or hand-enter a name.
func refreshPrinterWarning(combo *walk.ComboBox, label *walk.Label, printers *[]print.PrinterInfo) {
	if combo == nil || label == nil {
		return
	}
	text := combo.Text()
	if printers == nil {
		label.SetText("")
		return
	}
	p := findPrinterByLabel(*printers, text)
	if p != nil && p.IsWSD() {
		label.SetText("⚠ This printer is per-user (WSD). The daemon (LocalSystem) cannot reach it. Use \"Install network printer…\" to install machine-wide.")
	} else {
		label.SetText("")
	}
}

// showInstallPrinterDialog opens a sub-dialog for installing a TCP/IP printer
// machine-wide. onSuccess is invoked with the new printer's name once the
// install succeeds — the parent dialog uses that to refresh its dropdown and
// select the new entry.
func showInstallPrinterDialog(owner walk.Form, onSuccess func(installedName string)) {
	var sub *walk.Dialog
	var subStatus *walk.Label
	var hostEdit, nameEdit *walk.LineEdit
	var driverCombo *walk.ComboBox
	var installButton, cancelButton *walk.PushButton

	setSubStatus := func(text string) {
		if subStatus != nil {
			subStatus.SetText(text)
		}
	}

	err := (Dialog{
		AssignTo:      &sub,
		Title:         "Install network printer",
		MinSize:       Size{Width: 440, Height: 240},
		DefaultButton: &installButton,
		CancelButton:  &cancelButton,
		Layout:        VBox{},
		Children: []Widget{
			Label{Text: "Printer IP or hostname"},
			LineEdit{AssignTo: &hostEdit, CueBanner: "192.168.1.50"},
			Label{Text: "Driver (must be already installed on this machine)"},
			ComboBox{AssignTo: &driverCombo, Editable: false},
			Label{Text: "Printer name (how it appears in Windows)"},
			LineEdit{AssignTo: &nameEdit, CueBanner: "e.g. BADIR-NET"},
			Label{AssignTo: &subStatus},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{AssignTo: &installButton, Text: "Install", OnClicked: func() {
						host := strings.TrimSpace(hostEdit.Text())
						name := strings.TrimSpace(nameEdit.Text())
						driver := driverCombo.Text()
						if host == "" || name == "" || driver == "" {
							setSubStatus("All fields are required.")
							return
						}
						setSubStatus("Installing…")
						installButton.SetEnabled(false)
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
							defer cancel()
							err := print.InstallTCPPrinter(ctx, name, host, driver)
							sub.Synchronize(func() {
								installButton.SetEnabled(true)
								if err != nil {
									setSubStatus("Install failed: " + err.Error())
									return
								}
								setSubStatus("Installed.")
								if onSuccess != nil {
									onSuccess(name)
								}
								sub.Accept()
							})
						}()
					}},
					PushButton{AssignTo: &cancelButton, Text: "Cancel", OnClicked: func() {
						sub.Cancel()
					}},
				},
			},
		},
	}).Create(owner)
	if err != nil {
		walk.MsgBox(owner, "Error", "Failed to open install dialog: "+err.Error(), walk.MsgBoxIconError)
		return
	}

	// Populate driver list async so the dialog opens instantly.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		drivers, err := print.ListPrinterDrivers(ctx)
		sub.Synchronize(func() {
			if err != nil {
				setSubStatus("Could not list drivers: " + err.Error())
				return
			}
			_ = driverCombo.SetModel(drivers)
		})
	}()

	sub.Run()
}

// runTestPrint generates a tiny PDF via the existing chromedp pipeline and
// sends it to the selected printer through SumatraPDF. Surfaces the outcome
// in the parent dialog's status label. printerName "" means "system default"
// — same semantics as the runtime AfterOrder path.
func runTestPrint(dlg *walk.Dialog, setStatus func(string), printerName, chromePath, sumatraPath string, logSvc logger.LoggerService) {
	if chromePath == "" {
		chromePath = pdf.DetectChrome()
	}
	if chromePath == "" {
		dlg.Synchronize(func() { setStatus("Test print failed: Chrome not found — set Chrome Path or install Chrome.") })
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	gen := pdf.NewGenerator(chromePath)
	target := printerName
	if target == "" {
		target = "<system default>"
	}
	htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:sans-serif;padding:40px;">
<h1>erp-connector test print</h1>
<p>If you can read this page on paper, the PDF + print pipeline works end-to-end.</p>
<p><b>Time:</b> %s</p>
<p><b>Printer:</b> %s</p>
</body></html>`, html.EscapeString(time.Now().Format(time.RFC3339)), html.EscapeString(target))

	pdfBytes, err := gen.GenerateFromHTML(ctx, []byte(htmlBody))
	if err != nil {
		dlg.Synchronize(func() { setStatus("Test print: PDF render failed: " + err.Error()) })
		return
	}

	tmp, err := os.CreateTemp("", "erp-connector-testprint-*.pdf")
	if err != nil {
		dlg.Synchronize(func() { setStatus("Test print: temp file failed: " + err.Error()) })
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(pdfBytes); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		dlg.Synchronize(func() { setStatus("Test print: temp write failed: " + err.Error()) })
		return
	}
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := print.PrintPDF(ctx, tmpPath, printerName, sumatraPath, logSvc); err != nil {
		dlg.Synchronize(func() { setStatus("Test print failed: " + err.Error()) })
		return
	}
	dlg.Synchronize(func() {
		setStatus("Test print sent to " + target + ". Check the printer queue.")
	})
}
