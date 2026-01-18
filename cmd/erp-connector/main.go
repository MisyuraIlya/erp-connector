package main

import (
	"context"
	"erp-connector/internal/config"
	"erp-connector/internal/db"
	"erp-connector/internal/secrets"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
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

		container.NewHBox(testBtn, saveBtn),
		status,
	))

	w.Resize(fyne.NewSize(520, 690))
	w.ShowAndRun()
}
