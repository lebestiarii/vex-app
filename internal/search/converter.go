package search

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vex/internal/models"
)

var (
	// Matches: "1588 kg to lbs", "-30c in f", "4.3 cbm to ft3", "125 in = cm"
	unitConvRe = regexp.MustCompile(`(?i)^\s*(?:convert\s+)?([+-]?\d+(?:\.\d+)?)\s*([a-z°³3]+)\s+(?:to|in|into|as|=)\s+([a-z°³3]+)\s*$`)

	// Matches: "120x80x100 cm", "dim 48 x 40 x 60 in", "2 x 120x80x110 cm", "2pcs 120*80*100cm"
	dimBoxRe = regexp.MustCompile(`(?i)^\s*(?:dim|vol|cbm|box|cargo)?\s*(?:(\d+)\s*(?:pcs?|pallets?|boxes?|x|\*)\s*)?(\d+(?:\.\d+)?)\s*[x\*×]\s*(\d+(?:\.\d+)?)\s*[x\*×]\s*(\d+(?:\.\d+)?)\s*(cm|in|inch|inches|m|ft|feet)?\s*$`)
)

type unitCategory string

const (
	catWeight unitCategory = "weight"
	catLength unitCategory = "length"
	catVolume unitCategory = "volume"
	catTemp   unitCategory = "temp"
)

type unitDef struct {
	category unitCategory
	canonical string
	label     string
	toBase    float64 // base: kg for weight, m for length, m3 for volume
}

var unitTable = map[string]unitDef{
	// Weight (base: kg)
	"kg":        {catWeight, "kg", "kg", 1.0},
	"kgs":       {catWeight, "kg", "kg", 1.0},
	"kilo":      {catWeight, "kg", "kg", 1.0},
	"kilos":     {catWeight, "kg", "kg", 1.0},
	"kilogram":  {catWeight, "kg", "kg", 1.0},
	"kilograms": {catWeight, "kg", "kg", 1.0},
	"lb":        {catWeight, "lbs", "lbs", 0.45359237},
	"lbs":       {catWeight, "lbs", "lbs", 0.45359237},
	"pound":     {catWeight, "lbs", "lbs", 0.45359237},
	"pounds":    {catWeight, "lbs", "lbs", 0.45359237},
	"mt":        {catWeight, "mt", "Metric Tons (t)", 1000.0},
	"t":         {catWeight, "mt", "Metric Tons (t)", 1000.0},
	"ton":       {catWeight, "mt", "Metric Tons (t)", 1000.0},
	"tons":      {catWeight, "mt", "Metric Tons (t)", 1000.0},
	"tonne":     {catWeight, "mt", "Metric Tons (t)", 1000.0},
	"tonnes":    {catWeight, "mt", "Metric Tons (t)", 1000.0},
	"g":         {catWeight, "g", "g", 0.001},
	"gram":      {catWeight, "g", "g", 0.001},
	"grams":     {catWeight, "g", "g", 0.001},
	"oz":        {catWeight, "oz", "oz", 0.0283495231},
	"ounce":     {catWeight, "oz", "oz", 0.0283495231},
	"ounces":    {catWeight, "oz", "oz", 0.0283495231},

	// Length (base: m)
	"m":           {catLength, "m", "m", 1.0},
	"meter":       {catLength, "m", "m", 1.0},
	"meters":      {catLength, "m", "m", 1.0},
	"cm":          {catLength, "cm", "cm", 0.01},
	"centimeter":  {catLength, "cm", "cm", 0.01},
	"centimeters": {catLength, "cm", "cm", 0.01},
	"mm":          {catLength, "mm", "mm", 0.001},
	"in":          {catLength, "in", "in", 0.0254},
	"inch":        {catLength, "in", "in", 0.0254},
	"inches":      {catLength, "in", "in", 0.0254},
	"ft":          {catLength, "ft", "ft", 0.3048},
	"foot":        {catLength, "ft", "ft", 0.3048},
	"feet":        {catLength, "ft", "ft", 0.3048},
	"km":          {catLength, "km", "km", 1000.0},
	"mi":          {catLength, "mi", "miles", 1609.344},
	"mile":        {catLength, "mi", "miles", 1609.344},
	"miles":       {catLength, "mi", "miles", 1609.344},

	// Volume (base: m3 / CBM)
	"cbm":    {catVolume, "CBM", "CBM (m³)", 1.0},
	"m3":     {catVolume, "CBM", "CBM (m³)", 1.0},
	"m³":     {catVolume, "CBM", "CBM (m³)", 1.0},
	"cum":    {catVolume, "CBM", "CBM (m³)", 1.0},
	"ft3":    {catVolume, "ft³", "ft³ (cu ft)", 0.0283168466},
	"ft³":    {catVolume, "ft³", "ft³ (cu ft)", 0.0283168466},
	"cft":    {catVolume, "ft³", "ft³ (cu ft)", 0.0283168466},
	"cuft":   {catVolume, "ft³", "ft³ (cu ft)", 0.0283168466},
	"l":      {catVolume, "L", "Liters", 0.001},
	"liter":  {catVolume, "L", "Liters", 0.001},
	"liters": {catVolume, "L", "Liters", 0.001},
	"gal":    {catVolume, "gal", "US Gallons", 0.00378541178},
	"gallon": {catVolume, "gal", "US Gallons", 0.00378541178},

	// Temperature
	"c":          {catTemp, "°C", "°C", 1.0},
	"°c":         {catTemp, "°C", "°C", 1.0},
	"celsius":    {catTemp, "°C", "°C", 1.0},
	"f":          {catTemp, "°F", "°F", 1.0},
	"°f":         {catTemp, "°F", "°F", 1.0},
	"fahrenheit": {catTemp, "°F", "°F", 1.0},
}

// TryConvertQuery evaluates rawText for an instant unit conversion or 3D cargo dimensional
// weight calculation (e.g. "1588 kg to lbs", "-30c to f", "120x80x100 cm").
// Returns nil if rawText is not a conversion or calculation query.
func TryConvertQuery(rawText string) *models.SearchCard {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return nil
	}

	if card := tryParseUnitConversion(trimmed); card != nil {
		return card
	}
	if card := tryParseDimensionalBox(trimmed); card != nil {
		return card
	}
	return nil
}

func tryParseUnitConversion(text string) *models.SearchCard {
	m := unitConvRe.FindStringSubmatch(text)
	if len(m) != 4 {
		return nil
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	fromKey := strings.ToLower(m[2])
	toKey := strings.ToLower(m[3])

	fromDef, okFrom := unitTable[fromKey]
	toDef, okTo := unitTable[toKey]
	if !okFrom || !okTo || fromDef.category != toDef.category {
		return nil
	}

	var converted float64
	if fromDef.category == catTemp {
		if fromDef.canonical == "°C" && toDef.canonical == "°F" {
			converted = val*9.0/5.0 + 32.0
		} else if fromDef.canonical == "°F" && toDef.canonical == "°C" {
			converted = (val - 32.0) * 5.0 / 9.0
		} else {
			converted = val
		}
	} else {
		baseVal := val * fromDef.toBase
		converted = baseVal / toDef.toBase
	}

	formattedIn := formatNum(val) + " " + fromDef.canonical
	formattedOut := formatNum(converted) + " " + toDef.canonical

	data := map[string]any{
		"input":  formattedIn,
		"result": formattedOut,
	}
	pills := []string{formattedOut, formattedIn}

	equivalents := []map[string]string{}
	// Add bonus freight-relevant equivalents
	if fromDef.category == catWeight {
		baseKg := val * fromDef.toBase
		kgStr := formatNum(baseKg) + " kg"
		lbStr := formatNum(baseKg/0.45359237) + " lbs"
		mtStr := formatNum(baseKg/1000.0) + " t"
		data["kilograms"] = kgStr
		data["pounds"] = lbStr
		data["metric_tons"] = mtStr
		equivalents = append(equivalents,
			map[string]string{"label": "Kilograms", "value": kgStr},
			map[string]string{"label": "Pounds", "value": lbStr},
			map[string]string{"label": "Metric Tons", "value": mtStr},
		)
	} else if fromDef.category == catVolume {
		baseCBM := val * fromDef.toBase
		airVolKg := baseCBM * 166.6667 // IATA 1:167 kg/CBM (6000 cm3/kg)
		cbmStr := formatNum(baseCBM) + " CBM"
		ft3Str := formatNum(baseCBM/0.0283168466) + " ft³"
		airKgStr := formatNum(airVolKg) + " kg"
		data["cbm"] = cbmStr
		data["cubic_feet"] = ft3Str
		data["air_volumetric_weight"] = fmt.Sprintf("%s (%s lbs) @ 167 kg/CBM", airKgStr, formatNum(airVolKg*2.20462262))
		pills = append(pills, "Air Vol: "+airKgStr)
		equivalents = append(equivalents,
			map[string]string{"label": "CBM", "value": cbmStr},
			map[string]string{"label": "Cubic Feet", "value": ft3Str},
			map[string]string{"label": "Air Vol (1:167)", "value": airKgStr},
		)
	}
	data["output"] = formattedOut
	data["category"] = string(fromDef.category)
	data["equivalents"] = equivalents

	return &models.SearchCard{
		ID:              "builtin-unit-convert",
		Title:           fmt.Sprintf("%s = %s", formattedIn, formattedOut),
		Subtitle:        "Instant Unit & Freight Conversion",
		Description:     fmt.Sprintf("Converted %s (%s) to %s.", formattedIn, fromDef.label, formattedOut),
		Collection:      "system",
		Template:        models.TemplateConversion,
		Icon:            "⇄",
		Tags:            []string{"converter", "calculator"},
		DefaultExpanded: true,
		Chips: []models.GlanceChip{
			{Label: strings.ToUpper(string(fromDef.category)), Tone: "blue"},
			{Label: formattedOut, Tone: "green"},
			{Label: formattedIn, Tone: "neutral"},
		},
		GlancePills: pills,
		Data:        data,
		Actions: []models.CardAction{
			{ID: "copy-result", Label: "Copy " + formattedOut, Shortcut: "Enter", Type: "copy", Target: formattedOut},
			{ID: "copy-number", Label: "Copy Raw Number", Type: "copy", Target: formatNum(converted)},
		},
		UpdatedAt: time.Now().UTC(),
	}
}

func tryParseDimensionalBox(text string) *models.SearchCard {
	m := dimBoxRe.FindStringSubmatch(text)
	if len(m) != 6 {
		return nil
	}
	qty := 1
	if m[1] != "" {
		if q, err := strconv.Atoi(m[1]); err == nil && q > 0 {
			qty = q
		}
	}
	l, err1 := strconv.ParseFloat(m[2], 64)
	w, err2 := strconv.ParseFloat(m[3], 64)
	h, err3 := strconv.ParseFloat(m[4], 64)
	if err1 != nil || err2 != nil || err3 != nil || l <= 0 || w <= 0 || h <= 0 {
		return nil
	}

	unit := strings.ToLower(m[5])
	if unit == "" {
		unit = "cm"
	}

	var toMeters float64
	var unitLabel string
	switch unit {
	case "in", "inch", "inches":
		toMeters = 0.0254
		unitLabel = "in"
	case "m":
		toMeters = 1.0
		unitLabel = "m"
	case "ft", "feet":
		toMeters = 0.3048
		unitLabel = "ft"
	default:
		toMeters = 0.01
		unitLabel = "cm"
	}

	unitCBM := (l * toMeters) * (w * toMeters) * (h * toMeters)
	totalCBM := unitCBM * float64(qty)
	totalCuFt := totalCBM / 0.0283168466

	// IATA standard air volumetric weight: 1 CBM = 166.67 kg (divisor 6000 cm³/kg or 366 in³/kg, 166 in³/lb)
	airVolKg := totalCBM * 166.6667
	airVolLbs := airVolKg * 2.20462262

	// Ocean LCL W/M standard: 1 CBM = 1,000 kg (1 Revenue Ton)
	oceanWM := totalCBM

	dimSummary := fmt.Sprintf("%s × %s × %s %s", formatNum(l), formatNum(w), formatNum(h), unitLabel)
	if qty > 1 {
		dimSummary = fmt.Sprintf("%d pcs × (%s)", qty, dimSummary)
	}

	cbmStr := formatNum(totalCBM) + " CBM"
	airStr := fmt.Sprintf("%s kg (%s lbs)", formatNum(airVolKg), formatNum(airVolLbs))
	copySummary := fmt.Sprintf("%s | Volume: %s (%s ft³) | Air Volumetric Wt: %s", dimSummary, cbmStr, formatNum(totalCuFt), airStr)

	return &models.SearchCard{
		ID:              "builtin-unit-convert",
		Title:           fmt.Sprintf("%s → %s · Air Vol: %s", dimSummary, cbmStr, airStr),
		Subtitle:        "Instant Cargo Volume (CBM) & IATA Air Dimensional Weight Calculator (1:167)",
		Description:     "Calculated total cubic volume (CBM / ft³), IATA air freight volumetric weight (6,000 cm³/kg · 166.67 kg/CBM), and Ocean LCL Revenue Tons (W/M).",
		Collection:      "system",
		Template:        models.TemplateConversion,
		Icon:            "📦",
		Tags:            []string{"cbm", "volumetric", "dimensions", "calculator"},
		DefaultExpanded: true,
		Chips: []models.GlanceChip{
			{Label: "VOLUME", Tone: "blue"},
			{Label: cbmStr, Tone: "green"},
			{Label: "Air: " + formatNum(airVolKg) + " kg", Tone: "amber"},
		},
		GlancePills: []string{cbmStr, "Air: " + formatNum(airVolKg) + " kg", formatNum(totalCuFt) + " ft³"},
		Data: map[string]any{
			"input":                 dimSummary,
			"output":                cbmStr,
			"category":              "volume",
			"dimensions":            dimSummary,
			"total_volume_cbm":      fmt.Sprintf("%s CBM (m³)", formatNum(totalCBM)),
			"total_volume_cuft":     fmt.Sprintf("%s ft³", formatNum(totalCuFt)),
			"air_volumetric_weight": fmt.Sprintf("%s kg / %s lbs (IATA 1:167)", formatNum(airVolKg), formatNum(airVolLbs)),
			"ocean_lcl_wm":          fmt.Sprintf("%s W/M (Revenue Tons @ 1 CBM = 1,000 kg)", formatNum(oceanWM)),
			"equivalents": []map[string]string{
				{"label": "Cubic Feet", "value": formatNum(totalCuFt) + " ft³"},
				{"label": "IATA Air Vol", "value": formatNum(airVolKg) + " kg"},
				{"label": "Ocean W/M", "value": formatNum(oceanWM) + " W/M"},
			},
		},
		Actions: []models.CardAction{
			{ID: "copy-summary", Label: "Copy Volume & Air Weight", Shortcut: "Enter", Type: "copy", Target: copySummary},
			{ID: "copy-cbm", Label: "Copy " + cbmStr, Type: "copy", Target: cbmStr},
		},
		UpdatedAt: time.Now().UTC(),
	}
}

func formatNum(v float64) string {
	if math.Abs(v-math.Round(v)) < 1e-6 {
		return strconv.FormatInt(int64(math.Round(v)), 10)
	}
	if math.Abs(v) >= 100 {
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 2, 64), "0"), ".")
}
