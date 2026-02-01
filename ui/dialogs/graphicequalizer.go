package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/ui/layouts"
	myTheme "github.com/dweymouth/supersonic/ui/theme"
	"github.com/dweymouth/supersonic/ui/util"
)

type GraphicEqualizer struct {
	widget.BaseWidget

	OnChanged            func(band int, gain float64)
	OnPreampChanged      func(gain float64)
	OnLoadAutoEQProfile  func()
	OnManualAdjustment   func() // Called when user manually changes a slider

	bandSliders      []*eqSlider
	preampSlider     *eqSlider
	presetSelect     *widget.Select
	autoEQBtn        *widget.Button
	profileLabel     *widget.Label
	container        *fyne.Container
	eqPresets        []backend.EQPreset
	presetManager    *backend.EQPresetManager
	parentWindow     fyne.Window
	isApplyingPreset bool // Flag to prevent clearing profile during preset application
}

func NewGraphicEqualizer(preamp float64, bandFreqs []string, bandGains []float64, presetMgr *backend.EQPresetManager, parentWindow fyne.Window) *GraphicEqualizer {
	g := &GraphicEqualizer{
		presetManager: presetMgr,
		parentWindow:  parentWindow,
	}
	g.ExtendBaseWidget(g)
	g.loadPresets()
	g.buildSliders(preamp, bandFreqs, bandGains)

	return g
}

func (g *GraphicEqualizer) loadPresets() {
	presets, err := g.presetManager.LoadPresets()
	if err != nil {
		// Fallback to empty list if load fails
		g.eqPresets = []backend.EQPreset{}
		return
	}
	g.eqPresets = presets
}

func (g *GraphicEqualizer) buildSliders(preamp float64, bands []string, bandGains []float64) {
	// Build preset selector
	g.updatePresetSelect()

	// Reset button
	resetBtn := widget.NewButton(lang.L("Reset"), func() {
		// Find and apply the "Flat" preset
		for _, p := range g.eqPresets {
			if p.Name == "Flat" {
				g.applyPreset(p)
				g.presetSelect.SetSelected(p.Name)
				break
			}
		}
	})

	// Save button
	saveBtn := widget.NewButton(lang.L("Save"), func() {
		g.showSavePresetDialog()
	})

	// Delete button
	deleteBtn := widget.NewButton(lang.L("Delete"), func() {
		g.showDeletePresetDialog()
	})

	// AutoEQ button
	g.autoEQBtn = widget.NewButton(lang.L("AutoEQ"), func() {
		if g.OnLoadAutoEQProfile != nil {
			g.OnLoadAutoEQProfile()
		}
	})

	// Profile label (hidden by default)
	g.profileLabel = widget.NewLabel("")
	g.profileLabel.Hide()

	// Set minimum width for preset dropdown
	g.presetSelect.Resize(fyne.NewSize(200, g.presetSelect.MinSize().Height))

	// Top bar with controls in two rows
	topBar := container.NewVBox(
		// First row: Preset selector and main controls
		container.NewHBox(
			widget.NewLabel(lang.L("EQ Preset:")),
			g.presetSelect,
			layout.NewSpacer(),
			saveBtn,
			deleteBtn,
			resetBtn,
		),
		// Second row: AutoEQ and profile label
		container.NewHBox(
			g.autoEQBtn,
			g.profileLabel,
		),
	)

	// Range labels
	rng := container.NewVBox(
		newCaptionTextSizeLabel("+12", fyne.TextAlignTrailing),
		layout.NewSpacer(),
		newCaptionTextSizeLabel("0 dB", fyne.TextAlignTrailing),
		layout.NewSpacer(),
		newCaptionTextSizeLabel("-12", fyne.TextAlignTrailing),
	)

	g.bandSliders = make([]*eqSlider, len(bands))
	bandSlidersCtr := container.New(layouts.NewGridLayoutWithColumnsAndPadding(len(bands)+2, -16))

	// Preamp slider
	pre := newCaptionTextSizeLabel(lang.L("EQ Preamp"), fyne.TextAlignCenter)
	g.preampSlider = newEQSlider()
	g.preampSlider.SetValue(preamp)
	g.preampSlider.OnChanged = func(f float64) {
		if g.OnPreampChanged != nil {
			g.OnPreampChanged(f)
		}
		g.preampSlider.UpdateToolTip()
		if !g.isApplyingPreset && g.OnManualAdjustment != nil {
			g.OnManualAdjustment()
		}
	}
	g.preampSlider.UpdateToolTip()
	bandSlidersCtr.Add(container.NewBorder(nil, pre, nil, nil, g.preampSlider))
	bandSlidersCtr.Add(container.NewBorder(nil, widget.NewLabel(""), nil, nil, rng))

	// Band sliders
	for i, band := range bands {
		s := newEQSlider()
		if i < len(bandGains) {
			s.SetValue(bandGains[i])
			s.UpdateToolTip()
		}
		_i := i
		s.OnChanged = func(f float64) {
			if g.OnChanged != nil {
				g.OnChanged(_i, f)
			}
			g.bandSliders[_i].UpdateToolTip()
			if !g.isApplyingPreset && g.OnManualAdjustment != nil {
				g.OnManualAdjustment()
			}
		}
		l := newCaptionTextSizeLabel(band, fyne.TextAlignCenter)
		c := container.NewBorder(nil, l, nil, nil, s)
		bandSlidersCtr.Add(c)
		g.bandSliders[i] = s
	}

	sliderArea := container.NewStack(
		container.NewBorder(nil, widget.NewLabel(""), nil, nil,
			container.NewBorder(nil, nil, util.NewHSpace(5), util.NewHSpace(5),
				container.NewVBox(
					layout.NewSpacer(),
					myTheme.NewThemedRectangle(theme.ColorNameInputBackground),
					layout.NewSpacer(),
				),
			),
		),
		bandSlidersCtr,
	)

	g.container = container.NewBorder(topBar, nil, nil, nil, sliderArea)
}

func (g *GraphicEqualizer) updatePresetSelect() {
	presetNames := make([]string, len(g.eqPresets))
	for i, p := range g.eqPresets {
		displayName := p.Name
		if !p.IsBuiltin {
			displayName = p.Name + " *" // Mark custom presets with asterisk
		}
		presetNames[i] = displayName
	}

	if g.presetSelect == nil {
		g.presetSelect = widget.NewSelect(presetNames, func(selected string) {
			// Remove asterisk marker if present
			cleanName := selected
			if len(selected) > 2 && selected[len(selected)-2:] == " *" {
				cleanName = selected[:len(selected)-2]
			}
			for _, p := range g.eqPresets {
				if p.Name == cleanName {
					g.applyPreset(p)
					break
				}
			}
		})
		g.presetSelect.PlaceHolder = lang.L("EQ Preset")
	} else {
		g.presetSelect.Options = presetNames
		g.presetSelect.Refresh()
	}
}

func (g *GraphicEqualizer) applyPreset(preset backend.EQPreset) {
	g.isApplyingPreset = true
	defer func() { g.isApplyingPreset = false }()

	// Apply preamp
	g.preampSlider.SetValue(preset.Preamp)
	g.preampSlider.UpdateToolTip()
	if g.OnPreampChanged != nil {
		g.OnPreampChanged(preset.Preamp)
	}

	// Apply band gains
	for i, gain := range preset.Bands {
		if i < len(g.bandSliders) {
			g.bandSliders[i].SetValue(gain)
			g.bandSliders[i].UpdateToolTip()
			if g.OnChanged != nil {
				g.OnChanged(i, gain)
			}
		}
	}
}

func (g *GraphicEqualizer) getCurrentSettings() backend.EQPreset {
	preset := backend.EQPreset{
		Preamp: g.preampSlider.Value,
	}
	for i, slider := range g.bandSliders {
		preset.Bands[i] = slider.Value
	}
	return preset
}

func (g *GraphicEqualizer) showSavePresetDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(lang.L("Preset name"))

	formDialog := dialog.NewForm(
		lang.L("Save Preset"),
		lang.L("Save"),
		lang.L("Cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(lang.L("Name"), nameEntry),
		},
		func(confirmed bool) {
			if !confirmed || nameEntry.Text == "" {
				return
			}

			preset := g.getCurrentSettings()
			preset.Name = nameEntry.Text
			preset.IsBuiltin = false

			if err := g.presetManager.SavePreset(preset); err != nil {
				dialog.ShowError(err, g.parentWindow)
				return
			}

			// Reload presets and update UI
			g.loadPresets()
			g.updatePresetSelect()

			// Select the newly saved preset
			g.presetSelect.SetSelected(preset.Name + " *")
		},
		g.parentWindow,
	)

	formDialog.Resize(fyne.NewSize(400, 150))
	formDialog.Show()
}

func (g *GraphicEqualizer) showDeletePresetDialog() {
	selected := g.presetSelect.Selected
	if selected == "" {
		dialog.ShowInformation(lang.L("No Preset Selected"), lang.L("Please select a preset to delete"), g.parentWindow)
		return
	}

	// Remove asterisk marker if present
	cleanName := selected
	if len(selected) > 2 && selected[len(selected)-2:] == " *" {
		cleanName = selected[:len(selected)-2]
	}

	// Find the preset
	var presetToDelete *backend.EQPreset
	for i, p := range g.eqPresets {
		if p.Name == cleanName {
			presetToDelete = &g.eqPresets[i]
			break
		}
	}

	if presetToDelete == nil || presetToDelete.IsBuiltin {
		dialog.ShowInformation(lang.L("Cannot Delete"), lang.L("Cannot delete builtin presets"), g.parentWindow)
		return
	}

	dialog.ShowConfirm(
		lang.L("Delete Preset"),
		fmt.Sprintf(lang.L("Delete preset '%s'?"), cleanName),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			if err := g.presetManager.DeletePreset(cleanName); err != nil {
				dialog.ShowError(err, g.parentWindow)
				return
			}

			// Reload presets and update UI
			g.loadPresets()
			g.updatePresetSelect()
			g.presetSelect.ClearSelected()
		},
		g.parentWindow,
	)
}

// SetProfileLabel displays the name of the applied AutoEQ profile
func (g *GraphicEqualizer) SetProfileLabel(profileName string) {
	if profileName == "" {
		g.profileLabel.SetText("")
		g.profileLabel.Hide()
	} else {
		g.profileLabel.SetText(fmt.Sprintf("%s: %s", lang.L("Profile"), profileName))
		g.profileLabel.Show()
	}
}

// ClearProfileLabel hides the profile label (called on manual adjustment)
func (g *GraphicEqualizer) ClearProfileLabel() {
	g.SetProfileLabel("")
}

func newCaptionTextSizeLabel(text string, alignment fyne.TextAlign) *widget.RichText {
	l := widget.NewRichTextWithText(text)
	ts := l.Segments[0].(*widget.TextSegment)
	ts.Style.SizeName = theme.SizeNameCaptionText
	ts.Style.Alignment = alignment
	return l
}

func (g *GraphicEqualizer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.container)
}

type eqSlider struct {
	ttwidget.Slider
}

func newEQSlider() *eqSlider {
	s := &eqSlider{
		Slider: ttwidget.Slider{
			Slider: widget.Slider{
				Orientation: widget.Vertical,
				Min:         -12,
				Max:         12,
				Step:        0.1,
			},
		},
	}
	s.UpdateToolTip()
	s.ExtendBaseWidget(s)
	return s
}

func (s *eqSlider) UpdateToolTip() {
	s.SetToolTip(fmt.Sprintf("%0.1f dB", s.Value))
}

// We implement our own double tapping so that the Tapped behavior
// can be triggered instantly.
func (s *eqSlider) DoubleTapped(e *fyne.PointEvent) {
	s.SetValue(0)
}
