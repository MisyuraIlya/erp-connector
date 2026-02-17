package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"erp-connector/internal/config"
	"erp-connector/internal/db"
	"erp-connector/internal/erp/hasavshevet"
	"erp-connector/internal/logger"
	"erp-connector/internal/platform/autostart"
	"erp-connector/internal/secrets"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const connectordWindowsServiceName = "erp-connectord"

func dbPasswordKey(erp config.ERPType) string {
	return "db_password_" + string(erp)
}

func resolveDBPassword(erp config.ERPType, entered string, required bool) (string, error) {
	if entered != "" {
		return entered, nil
	}
	if !required {
		return "", nil
	}
	b, err := secrets.Get(dbPasswordKey(erp))
	if err != nil {
		return "", fmt.Errorf("db password is required to initialize Hasavshevet procedures: %w", err)
	}
	return string(b), nil
}

func newBearerToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func findConnectordBinary() (string, error) {
	candidates := make([]string, 0, 4)
	searchDirs := make([]string, 0, 2)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		searchDirs = append(searchDirs, exeDir)
		candidates = append(candidates,
			filepath.Join(exeDir, "erp-connectord"),
			filepath.Join(exeDir, "erp-connectord.exe"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		searchDirs = append(searchDirs, wd)
		candidates = append(candidates,
			filepath.Join(wd, "erp-connectord"),
			filepath.Join(wd, "erp-connectord.exe"),
		)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("erp-connectord"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("erp-connectord.exe"); err == nil {
		return p, nil
	}
	for _, dir := range searchDirs {
		if dir == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, "erp-connectord*.exe"))
		if len(matches) == 0 {
			matches, _ = filepath.Glob(filepath.Join(dir, "erp-connectord*"))
		}
		if len(matches) == 0 {
			continue
		}
		best := matches[0]
		bestInfo, err := os.Stat(best)
		if err != nil {
			continue
		}
		bestTime := bestInfo.ModTime()
		for _, candidate := range matches[1:] {
			info, err := os.Stat(candidate)
			if err != nil {
				continue
			}
			if info.ModTime().After(bestTime) {
				best = candidate
				bestTime = info.ModTime()
			}
		}
		return best, nil
	}
	return "", fmt.Errorf("erp-connectord binary not found")
}

func main() {
	uiLog := newUILogger()
	defer uiLog.Close()
	defer func() {
		if rec := recover(); rec != nil {
			uiLog.Printf("panic: %v", rec)
			uiStartupAlert(fmt.Errorf("unexpected error; see UI log for details"))
		}
	}()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(newLogWatcher(uiLog.Writer(), handleOpenGLFailure))

	uiLog.Printf("startup begin")
	if session := os.Getenv("SESSIONNAME"); session != "" {
		uiLog.Printf("session: %s", session)
	}

	if handled, err := runHeadless(uiLog); handled {
		if err != nil {
			uiLog.Printf("headless error: %v", err)
			uiStartupAlert(err)
		}
		return
	}

	if err := uiStartupGuard(); err != nil {
		uiLog.Printf("startup blocked: %v", err)
		uiStartupAlert(err)
		return
	}

	if exe, err := os.Executable(); err == nil {
		uiLog.Printf("exe: %s", exe)
	}
	if wd, err := os.Getwd(); err == nil {
		uiLog.Printf("working dir: %s", wd)
	}

	uiLog.Printf("fyne app init")
	a := app.New()
	uiLog.Printf("fyne app ready")
	w := a.NewWindow("Digitrage Erp Connector")
	uiLog.Printf("window created")

	cfg, err := config.LoadOrDefault()
	if err == nil {
		uiLog.Printf("config loaded")
	} else {
		uiLog.Printf("config load error: %v", err)
	}
	logSvc, logErr := logger.New(cfg)
	if logErr != nil {
		logSvc = logger.NewStderr()
		logSvc.Warn("logger init failed; using stderr")
	}
	defer func() {
		_ = logSvc.Close()
	}()

	status := widget.NewLabel("")
	if err != nil {
		status.SetText("Error loading config: " + err.Error())
	}

	apiListenEntry := widget.NewEntry()
	apiListenEntry.SetText(cfg.APIListen)

	debugCheck := widget.NewCheck("Debug mode", nil)
	debugCheck.SetChecked(cfg.Debug)

	bearerTokenEntry := widget.NewEntry()
	bearerTokenEntry.SetText(cfg.BearerToken)
	bearerTokenBtn := widget.NewButton("Generate key", func() {
		token, err := newBearerToken()
		if err != nil {
			status.SetText("Failed to generate key: " + err.Error())
			return
		}
		bearerTokenEntry.SetText(token)
	})
	bearerTokenRow := container.NewBorder(nil, nil, nil, bearerTokenBtn, bearerTokenEntry)

	driverSelect := widget.NewSelect(config.DBDriverOptions(), func(string) {})
	driverSelect.SetSelected(string(cfg.DB.Driver))

	hostEntry := widget.NewEntry()
	hostEntry.SetText(cfg.DB.Host)

	portEntry := widget.NewEntry()
	portEntry.SetText(strconv.Itoa(cfg.DB.Port))

	userEntry := widget.NewEntry()
	userEntry.SetText(cfg.DB.User)

	dbEntry := widget.NewEntry()
	dbEntry.SetText(cfg.DB.Database)

	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Leave blank to keep existing")

	erpSelect := widget.NewSelect(config.ErpOption(), func(string) {})
	erpSelect.SetSelected(string(cfg.ERP))

	folderEntries := []*widget.Entry{}
	foldersBox := container.NewVBox()
	addFolderRow := func(path string) {
		entry := widget.NewEntry()
		entry.SetText(path)
		browseBtn := widget.NewButton("Browse", func() {
			dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
				if err != nil {
					status.SetText("Folder selection error: " + err.Error())
					return
				}
				if uri == nil {
					return
				}
				entry.SetText(uri.Path())
			}, w)
		})
		folderEntries = append(folderEntries, entry)
		foldersBox.Add(container.NewBorder(nil, nil, nil, browseBtn, entry))
		foldersBox.Refresh()
	}

	if len(cfg.ImageFolders) == 0 {
		addFolderRow("")
	} else {
		for _, p := range cfg.ImageFolders {
			addFolderRow(p)
		}
	}

	addFolderBtn := widget.NewButton("Add new folder path", func() {
		addFolderRow("")
	})

	sendOrderEntry := widget.NewEntry()
	sendOrderEntry.SetText(cfg.SendOrderDir)
	sendOrderBrowseBtn := widget.NewButton("Browse", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				status.SetText("Folder selection error: " + err.Error())
				return
			}
			if uri == nil {
				return
			}
			sendOrderEntry.SetText(uri.Path())
		}, w)
	})
	sendOrderRow := container.NewBorder(nil, nil, nil, sendOrderBrowseBtn, sendOrderEntry)
	sendOrderBox := container.NewVBox(
		widget.NewLabel("Send order folder"),
		sendOrderRow,
	)

	updateSendOrderVisibility := func(erp config.ERPType) {
		if erp == config.ERPHasavshevet {
			sendOrderBox.Show()
			return
		}
		sendOrderBox.Hide()
	}
	erpSelect.OnChanged = func(selected string) {
		updateSendOrderVisibility(config.ERPType(selected))
	}
	updateSendOrderVisibility(cfg.ERP)

	testBtn := widget.NewButton("Test connection", func() {
		tmp := cfg
		tmp.ERP = config.ERPType(erpSelect.Selected)
		tmp.APIListen = apiListenEntry.Text
		tmp.DB.Driver = config.DBDriver(driverSelect.Selected)
		tmp.DB.Host = hostEntry.Text

		p, err := strconv.Atoi(portEntry.Text)
		if err != nil || p <= 0 || p > 65535 {
			status.SetText("Invalid DB Port")
			return
		}

		tmp.DB.Port = p
		tmp.DB.User = userEntry.Text
		tmp.DB.Database = dbEntry.Text

		status.SetText("Testing connection...")
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		errTest := db.TestConnection(ctx, tmp, passEntry.Text)
		if errTest != nil {
			status.SetText("Conntextion faild:" + errTest.Error())
			return
		}

		status.SetText("Connection OK")
	})

	saveConfig := func() error {
		cfg.ERP = config.ERPType(erpSelect.Selected)
		cfg.APIListen = apiListenEntry.Text
		cfg.Debug = debugCheck.Checked
		cfg.BearerToken = strings.TrimSpace(bearerTokenEntry.Text)
		cfg.DB.Driver = config.DBDriver(driverSelect.Selected)
		cfg.DB.Host = hostEntry.Text

		p, err := strconv.Atoi(portEntry.Text)

		if err != nil || p <= 0 || p > 65535 {
			return fmt.Errorf("Invalid DB PORT")
		}

		cfg.DB.Port = p
		cfg.DB.User = userEntry.Text
		cfg.DB.Database = dbEntry.Text

		if cfg.ERP == config.ERPHasavshevet && strings.TrimSpace(cfg.DB.Database) == "" {
			return fmt.Errorf("DB database is required for Hasavshevet")
		}

		imageFolders := make([]string, 0, len(folderEntries))
		for _, entry := range folderEntries {
			path := strings.TrimSpace(entry.Text)
			if path == "" {
				continue
			}
			imageFolders = append(imageFolders, path)
		}
		cfg.ImageFolders = imageFolders

		if cfg.ERP == config.ERPHasavshevet {
			cfg.SendOrderDir = strings.TrimSpace(sendOrderEntry.Text)
		} else {
			cfg.SendOrderDir = ""
		}

		password, err := resolveDBPassword(cfg.ERP, passEntry.Text, cfg.ERP == config.ERPHasavshevet)
		if err != nil {
			return err
		}

		if cfg.ERP == config.ERPHasavshevet {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()

			dbConn, err := db.Open(cfg, password, db.DefaultOptions())
			if err != nil {
				return fmt.Errorf("failed to connect for Hasavshevet procedure setup: %w", err)
			}
			defer dbConn.Close()

			created, err := hasavshevet.EnsureGPriceBulkProcedure(ctx, dbConn)
			if err != nil {
				logSvc.Error("failed to initialize GPRICE_Bulk", err)
				return fmt.Errorf("failed to initialize GPRICE_Bulk: %w", err)
			}
			if created {
				logSvc.Success("GPRICE_Bulk created")
			} else {
				logSvc.Info("GPRICE_Bulk already exists")
			}

			created, err = hasavshevet.EnsureOnHandStockForSkusProcedure(ctx, dbConn)
			if err != nil {
				logSvc.Error("failed to initialize GetOnHandStockForSkus", err)
				return fmt.Errorf("failed to initialize GetOnHandStockForSkus: %w", err)
			}
			if created {
				logSvc.Success("GetOnHandStockForSkus created")
			} else {
				logSvc.Info("GetOnHandStockForSkus already exists")
			}
		}

		if passEntry.Text != "" {
			errPass := secrets.Set(dbPasswordKey(cfg.ERP), []byte(passEntry.Text))
			if errPass != nil {
				return fmt.Errorf("failed to save password: %s", errPass.Error())
			}
		}

		errSave := config.Save(cfg)
		if errSave != nil {
			return fmt.Errorf("Error saving config: %s", errSave.Error())
		}
		return nil
	}

	saveBtn := widget.NewButton("שמירה", func() {
		if err := saveConfig(); err != nil {
			status.SetText(err.Error())
			return
		}
		status.SetText("נשמר בהצלחה.")
	})

	startServerBtn := widget.NewButton("Start server", func() {
		if err := saveConfig(); err != nil {
			status.SetText(err.Error())
			return
		}
		daemonPath, err := findConnectordBinary()
		if err != nil {
			status.SetText("Start failed: " + err.Error())
			return
		}
		if runtime.GOOS == "windows" {
			created, err := autostart.EnsureWindowsServiceAutoStart(connectordWindowsServiceName, daemonPath)
			if err != nil {
				status.SetText("Failed to create/update server service: " + err.Error())
				return
			}
			if err := autostart.StartWindowsService(connectordWindowsServiceName); err != nil {
				status.SetText("Failed to start server service: " + err.Error())
				return
			}
			if created {
				status.SetText("Server service created and started.")
			} else {
				status.SetText("Server service started.")
			}
			return
		}

		cmd := exec.Command(daemonPath)
		if err := cmd.Start(); err != nil {
			status.SetText("Failed to start server: " + err.Error())
			return
		}
		if err := cmd.Process.Release(); err != nil {
			status.SetText("Server started, but release failed: " + err.Error())
			return
		}
		status.SetText("Server started.")
	})

	stopServerBtn := widget.NewButton("Stop server", func() {
		if runtime.GOOS != "windows" {
			status.SetText("Stop server service is supported on Windows only.")
			return
		}
		if err := autostart.StopWindowsService(connectordWindowsServiceName, 20*time.Second); err != nil {
			status.SetText("Failed to stop server service: " + err.Error())
			return
		}
		status.SetText("Server service stopped.")
	})

	content := container.NewVBox(
		widget.NewLabel("ERP"),
		erpSelect,

		widget.NewLabel("API Listen (host:port)"),
		apiListenEntry,
		debugCheck,
		widget.NewLabel("Bearer token"),
		bearerTokenRow,

		widget.NewSeparator(),
		widget.NewLabel("DB Settings"),
		widget.NewLabel("Driver"),
		driverSelect,
		widget.NewLabel("Host"),
		hostEntry,
		widget.NewLabel("Port"),
		portEntry,
		widget.NewLabel("User"),
		userEntry,
		widget.NewLabel("Database"),
		dbEntry,
		widget.NewLabel("Password"),
		passEntry,

		widget.NewSeparator(),
		widget.NewLabel("Image folders"),
		foldersBox,
		addFolderBtn,
		sendOrderBox,

		container.NewHBox(testBtn, saveBtn, startServerBtn, stopServerBtn),
		status,
	)

	scroll := container.NewVScroll(content)
	w.SetContent(scroll)

	w.Resize(fyne.NewSize(520, 690))
	w.SetFixedSize(false)
	uiLog.Printf("show window")
	w.Show()
	w.CenterOnScreen()
	w.RequestFocus()
	uiLog.Printf("run loop")
	a.Run()
	uiLog.Printf("run loop exit")
}
