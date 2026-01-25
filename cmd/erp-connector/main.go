package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"erp-connector/internal/config"
	"erp-connector/internal/db"
	"erp-connector/internal/secrets"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func dbPasswordKey(erp config.ERPType) string {
	return "db_password_" + string(erp)
}

func newBearerToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func main() {
	a := app.New()
	w := a.NewWindow("Digitrage Erp Connector")

	cfg, err := config.LoadOrDefault()
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

	saveBtn := widget.NewButton("שמירה", func() {

		cfg.ERP = config.ERPType(erpSelect.Selected)
		cfg.APIListen = apiListenEntry.Text
		cfg.Debug = debugCheck.Checked
		cfg.BearerToken = strings.TrimSpace(bearerTokenEntry.Text)
		cfg.DB.Driver = config.DBDriver(driverSelect.Selected)
		cfg.DB.Host = hostEntry.Text

		p, err := strconv.Atoi(portEntry.Text)

		if err != nil || p <= 0 || p > 65535 {
			status.SetText("Invalid DB PORT")
			return
		}

		cfg.DB.Port = p
		cfg.DB.User = userEntry.Text
		cfg.DB.Database = dbEntry.Text

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

		if passEntry.Text != "" {
			errPass := secrets.Set(dbPasswordKey(cfg.ERP), []byte(passEntry.Text))
			if errPass != nil {
				status.SetText("failed to save password: " + errPass.Error())
				return
			}
		}

		errSave := config.Save(cfg)
		if errSave != nil {
			status.SetText("Error saving config: " + errSave.Error())
			return
		}

		status.SetText("נשמר בהצלחה.")
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

		container.NewHBox(testBtn, saveBtn),
		status,
	)

	scroll := container.NewVScroll(content)
	w.SetContent(scroll)

	w.Resize(fyne.NewSize(520, 690))
	w.SetFixedSize(false)
	w.ShowAndRun()
}
