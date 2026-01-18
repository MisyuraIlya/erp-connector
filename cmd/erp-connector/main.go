package main

import (
	"erp-connector/internal/config"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	a := app.New()
	w := a.NewWindow("Digitrage Erp Connector")

	cfg, err := config.LoadOrDefault()
	status := widget.NewLabel("")
	if err != nil {
		status.SetText("Error loading config: " + err.Error())
	}

	erpSelect := widget.NewSelect(config.ErpOption(), func(string) {})
	erpSelect.SetSelected(string(cfg.ERP))

	apiListenEnty := widget.NewEntry()
	apiListenEnty.SetText(cfg.APIListen)

	saveBtn := widget.NewButton("שמירה", func() {
		cfg.ERP = config.ERPType(erpSelect.Selected)
		cfg.APIListen = apiListenEnty.Text

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
		apiListenEnty,
		saveBtn,
		status,
	))

	w.Resize(fyne.NewSize(420, 240))
	w.ShowAndRun()
}
