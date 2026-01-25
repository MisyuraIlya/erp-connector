package main

import (
	"context"
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
		foldersBox.Add(container.NewHBox(entry, browseBtn))
		foldersBox.Refresh()
	}

	if len(cfg.ImageFolders) == 0 {
		addFolderRow("")
	} else {
		for _, p := range cfg.ImageFolders {
			addFolderRow(p)
		}
	}

	addFolderBtn := widget.NewButton("Add folder", func() {
		addFolderRow("")
	})

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

		errPass := secrets.Set(dbPasswordKey(cfg.ERP), []byte(passEntry.Text))
		if errPass != nil {
			status.SetText("failed to save password: " + errPass.Error())
			return
		}

		errSave := config.Save(cfg)
		if errSave != nil {
			status.SetText("Error saving config: " + errSave.Error())
			return
		}

		status.SetText("נשמר בהצלחה.")
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("ERP"),
		erpSelect,

		widget.NewLabel("API Listen (host:port)"),
		apiListenEntry,

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

		container.NewHBox(testBtn, saveBtn),
		status,
	))

	w.Resize(fyne.NewSize(520, 690))
	w.ShowAndRun()
}
