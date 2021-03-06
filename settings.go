/**
 *Copyright (c) 2018 Stephen Merrony
 *
 *This software is released under the MIT License.
 *https://opensource.org/licenses/MIT
 */

package main

import (
	"io/ioutil"
	"log"

	"github.com/mattn/go-gtk/gtk"
	"gopkg.in/yaml.v2"
)

// settings holds the settings we want to persist across program invocations
type settingsT struct {
	JoystickID   int
	JoystickType string
	DataDir      string
	WideVideo    bool
}

func saveSettings(s settingsT, filename string) error {
	bytes, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, bytes, 0644)
}

func loadSettings(filename string) (settingsT, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return settingsT{}, err
	}
	var s settingsT
	err = yaml.Unmarshal(bytes, &s)
	if err != nil {
		return settingsT{}, err
	}
	return s, nil
}

func settingsCB() {
	sd := gtk.NewDialog()
	sd.SetTitle(appName + " Settings")
	sd.SetIcon(iconPixbuf)
	sd.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)

	table := gtk.NewTable(6, 3, false)
	table.SetColSpacings(5)
	table.SetRowSpacings(5)

	table.AttachDefaults(gtk.NewLabel("Detected"), 1, 2, 0, 1)
	table.AttachDefaults(gtk.NewLabel("Type"), 2, 3, 0, 1)
	jsLab := gtk.NewLabel("Joystick :")
	jsLab.SetAlignment(1, 0.5)
	table.AttachDefaults(jsLab, 0, 1, 1, 2)

	// display all joysticks detected on the system
	foundCombo := gtk.NewComboBoxText()
	found := listJoysticks()
	for _, j := range found {
		foundCombo.AppendText(j.Name)
	}
	if settingsLoaded {
		foundCombo.SetActive(settings.JoystickID)
	}
	table.AttachDefaults(foundCombo, 1, 2, 1, 2)

	// display all known joystick types
	chosenTypeCombo := gtk.NewComboBoxText()
	known := listKnownJoystickTypes()
	for i, k := range known {
		chosenTypeCombo.AppendText(k.Name)
		if settings.JoystickType == k.Name {
			chosenTypeCombo.SetActive(i)
		}
	}
	table.AttachDefaults(chosenTypeCombo, 2, 3, 1, 2)

	ddLab := gtk.NewLabel("Data Directory :")
	ddLab.SetAlignment(1, 0.5)
	table.AttachDefaults(ddLab, 0, 1, 2, 3)
	if settings.DataDir == "" {
		settings.DataDir = "."
	}
	ddLabel := gtk.NewLabel(settings.DataDir)
	ddLabel.SetAlignment(-1, 0.5)
	table.AttachDefaults(ddLabel, 1, 2, 2, 3)
	cdirBtn := gtk.NewButtonWithLabel("Change Dir.")
	table.AttachDefaults(cdirBtn, 2, 3, 2, 3)
	cdirBtn.Connect("clicked", func() {
		dc := gtk.NewFileChooserDialog(
			"Directory for Data Files",
			win, gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER, "_Cancel", gtk.RESPONSE_CANCEL, "_OK", gtk.RESPONSE_ACCEPT)
		dc.SetCurrentFolder(settings.DataDir)
		res := dc.Run()
		if res == gtk.RESPONSE_ACCEPT {
			settings.DataDir = dc.GetCurrentFolder()
			ddLabel.SetText(settings.DataDir)
		}
		dc.Destroy()
	})

	vmLab := gtk.NewLabel("Video Mode :")
	vmLab.SetAlignment(1, 0.5)
	table.AttachDefaults(vmLab, 0, 1, 3, 4)
	vm := gtk.NewCheckButtonWithLabel("Wide")
	if settings.WideVideo {
		vm.SetActive(true)
	}
	table.AttachDefaults(vm, 1, 2, 3, 4)

	sd.GetVBox().PackStart(table, true, true, 5)
	sd.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	sd.AddButton("OK", gtk.RESPONSE_OK)
	sd.SetDefaultResponse(gtk.RESPONSE_OK)
	sd.ShowAll()

	response := sd.Run()
	if response == gtk.RESPONSE_OK {
		settings.JoystickID = foundCombo.GetActive()
		settings.JoystickType = chosenTypeCombo.GetActiveText()
		settings.WideVideo = vm.GetActive()
		if err := saveSettings(settings, appSettingsFile); err != nil {
			messageDialog(win, gtk.MESSAGE_ERROR, "Could not save settings.")
			log.Printf("Could not save settings: %v", err)
		} else {
			messageDialog(win, gtk.MESSAGE_INFO, `Settings Saved
		
N.B. If you changed Joystick settings either
reconnect to the drone or restart the program.

If you changed video mode please restart the program.`)
		}
	}
	sd.Destroy()
}
