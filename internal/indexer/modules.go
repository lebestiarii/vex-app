package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vex/internal/models"
)

const CompiledModulesSource = "module://compiled"

type gatewaySpec struct {
	Code       string
	UNLOCODE   string
	ICAO       string
	City       string
	Airport    string
	Country    string
	ISO2       string
	Currency   string
	Timezone   string
	Region     string
	Role       string
	PortType   string
	Lat        float64
	Lon        float64
	AirCargo   string
	OceanNote  string
	Customs    string
	TechServer string
	TechSubnet string
}

var majorGateways = []gatewaySpec{
	{
		Code: "AMS", UNLOCODE: "NLAMS", ICAO: "EHAM", City: "Amsterdam", Airport: "Amsterdam Airport Schiphol",
		Country: "Netherlands", ISO2: "NL", Currency: "EUR (€)", Timezone: "Europe/Amsterdam", Region: "EMEA",
		PortType: "Air Cargo Hub & Seaport", Lat: 52.3105, Lon: 4.7683,
		Role:       "Major European air cargo gateway, pharma cold-chain center, and North Sea Canal seaport with active logistics branch office.",
		AirCargo:   "Schiphol Cargo World (Southeast) • 6 runways • GDP Pharma & Perishables Center • Full B747-8F / B777F main-deck capability",
		OceanNote:  "Port of Amsterdam (North Sea Canal) + 45-min barge/truck link to Port of Rotterdam (NLRTM)",
		Customs:    "Douane Netherlands • EU ICS2 Advance Filing • Bonded Schiphol Freezone & VAT Deferment (Article 23)",
		TechServer: "ams-prd-edi01.corp.internal", TechSubnet: "10.44.12.0/24",
	},
	{
		Code: "SEA", UNLOCODE: "USSEA", ICAO: "KSEA", City: "Seattle", Airport: "Seattle-Tacoma International Airport",
		Country: "United States", ISO2: "US", Currency: "USD ($)", Timezone: "America/Los_Angeles", Region: "North America",
		PortType: "Air & Ocean Gateway (HQ Proximity)", Lat: 47.4502, Lon: -122.3088,
		Role:       "Primary Pacific Northwest air and ocean gateway serving Pacific Rim air/ocean lanes.",
		AirCargo:   "Sea-Tac North Cargo Complex • Direct trans-Pacific widebody & freighter lift to APAC",
		OceanNote:  "Northwest Seaport Alliance (Port of Seattle & Tacoma) • Deep-water container terminals T-5, T-18, T-46",
		Customs:    "U.S. CBP Port Code 3001 (Seattle) • ACE eManifest, ISF (10+2), and FDA/USDA inspection station",
		TechServer: "sea-prd-core01.corp.internal", TechSubnet: "10.10.0.0/22",
	},
	{
		Code: "FRA", UNLOCODE: "DEFRA", ICAO: "EDDF", City: "Frankfurt", Airport: "Frankfurt Airport (CargoCity Süd)",
		Country: "Germany", ISO2: "DE", Currency: "EUR (€)", Timezone: "Europe/Berlin", Region: "EMEA",
		PortType: "Primary European Air Hub", Lat: 50.0379, Lon: 8.5622,
		Role:       "Flagship European air freight consolidation hub located in CargoCity Süd, connecting global trade lanes across Central Europe.",
		AirCargo:   "CargoCity Süd & Nord • Europe's busiest air freight hub • CEIV Pharma & dangerous goods (DGR) hub",
		Customs:    "German Zoll (ATLAS electronic customs) • EU ICS2 security filing & T1 transit bond gateway",
		TechServer: "fra-prd-edi01.corp.internal", TechSubnet: "10.49.20.0/24",
	},
	{
		Code: "HKG", UNLOCODE: "HKHKG", ICAO: "VHHH", City: "Hong Kong", Airport: "Hong Kong International Airport",
		Country: "Hong Kong", ISO2: "HK", Currency: "HKD (HK$)", Timezone: "Asia/Hong_Kong", Region: "APAC",
		PortType: "World #1 Air Cargo & Seaport Hub", Lat: 22.3080, Lon: 113.9185,
		Role:       "Powerhouse APAC gateway branch anchoring intra-Asia and outbound trans-Pacific/Europe air consolidation and ocean transshipment.",
		AirCargo:   "SuperTerminal 1 (Hactl) & Cathay Cargo Terminal • World's highest-volume air freight airport",
		OceanNote:  "Kwai Tsing Container Terminals (Port of Hong Kong) • Pearl River Delta barge feeder network",
		TechServer: "hkg-prd-edi01.corp.internal", TechSubnet: "10.85.10.0/24",
	},
	{
		Code: "PVG", UNLOCODE: "CNSHA", ICAO: "ZSPD", City: "Shanghai", Airport: "Shanghai Pudong International Airport",
		Country: "China", ISO2: "CN", Currency: "CNY (¥)", Timezone: "Asia/Shanghai", Region: "APAC",
		PortType: "Major Air & #1 Global Container Seaport", Lat: 31.1443, Lon: 121.8083,
		Role:       "Flagship mainland China gateway branch supporting electronics, automotive, and industrial manufacturing exports via Pudong (PVG) and Yangshan Port (CNSHA).",
		AirCargo:   "PACTL & Eastern Air Logistics terminals • High-frequency B747-8F / B777F main-deck freighter departures",
		OceanNote:  "Port of Shanghai (CNSHA / Yangshan Deep-Water Port) • World's busiest container seaport by TEU",
		TechServer: "pvg-prd-edi01.corp.internal", TechSubnet: "10.86.16.0/24",
	},
	{
		Code: "LAX", UNLOCODE: "USLAX", ICAO: "KLAX", City: "Los Angeles", Airport: "Los Angeles International Airport & Port of LA",
		Country: "United States", ISO2: "US", Currency: "USD ($)", Timezone: "America/Los_Angeles", Region: "North America",
		PortType: "Premier US West Coast Air & Ocean Hub", Lat: 33.9416, Lon: -118.4085,
		Role:       "Premier US West Coast gateway branch handling trans-Pacific air freight at LAX and ocean container imports at San Pedro Bay (USLAX / USLGB).",
		AirCargo:   "LAX Century Cargo Complex • Trans-Pacific e-commerce, aerospace, and perishables hub",
		OceanNote:  "Port of Los Angeles (USLAX) & Long Beach (USLGB) • Alameda Corridor intermodal rail gateway",
		Customs:    "U.S. CBP Port Code 2704 (LA/Long Beach) & 2720 (LAX Airport) • AMS / ISF 10+2 enforcement hub",
		TechServer: "lax-prd-edi01.corp.internal", TechSubnet: "10.12.40.0/24",
	},
	{
		Code: "ORD", UNLOCODE: "USORD", ICAO: "KORD", City: "Chicago", Airport: "Chicago O'Hare International Airport",
		Country: "United States", ISO2: "US", Currency: "USD ($)", Timezone: "America/Chicago", Region: "North America",
		PortType: "Midwest US Air & Intermodal Gateway", Lat: 41.9742, Lon: -87.9073,
		Role:       "Major US Midwest gateway branch at O'Hare Northeast Cargo Center, serving as a central consolidation and road-feeder hub.",
		AirCargo:   "O'Hare Northeast Cargo Campus • Direct main-deck freighters from Europe and Asia",
		Customs:    "U.S. CBP Port Code 3901 (Chicago) • Central US customs brokerage & in-bond (IT/IE) hub",
		TechServer: "ord-prd-edi01.corp.internal", TechSubnet: "10.14.20.0/24",
	},
	{
		Code: "SIN", UNLOCODE: "SGSIN", ICAO: "WSSS", City: "Singapore", Airport: "Singapore Changi Airport & PSA Port",
		Country: "Singapore", ISO2: "SG", Currency: "SGD (S$)", Timezone: "Asia/Singapore", Region: "APAC",
		PortType: "SE Asia Regional HQ, Air & Ocean Hub", Lat: 1.3644, Lon: 103.9915,
		Role:       "Southeast Asia regional headquarters and strategic multimodal transshipment gateway (Changi Alps Free Trade Zone & Tuas Mega Port).",
		AirCargo:   "Changi Airfreight Centre (CAC / Alps Ave FTZ) • SATS & dnata cool-chain pharma facilities",
		OceanNote:  "Port of Singapore (SGSIN / PSA Tuas & Pasir Panjang) • World's largest container transshipment hub",
		TechServer: "sin-prd-edi01.corp.internal", TechSubnet: "10.65.10.0/24",
	},
	{
		Code: "JFK", UNLOCODE: "USJFK", ICAO: "KJFK", City: "New York", Airport: "John F. Kennedy International Airport",
		Country: "United States", ISO2: "US", Currency: "USD ($)", Timezone: "America/New_York", Region: "North America",
		PortType: "Primary US East Coast Air Gateway", Lat: 40.6413, Lon: -73.7781,
		Role:       "Flagship US East Coast gateway branch handling high-velocity trans-Atlantic air cargo and NY/NJ Port Authority shipments.",
		AirCargo:   "JFK Cargo Plaza • Primary trans-Atlantic fashion, pharma, and high-value air gateway",
		Customs:    "U.S. CBP Port Code 4701 (JFK Airport) & 1001 (NY/Newark Seaport)",
	},
	{
		Code: "MIA", UNLOCODE: "USMIA", ICAO: "KMIA", City: "Miami", Airport: "Miami International Airport",
		Country: "United States", ISO2: "US", Currency: "USD ($)", Timezone: "America/New_York", Region: "North America",
		PortType: "LATAM & Perishables Air Gateway", Lat: 25.7959, Lon: -80.2870,
		Role:       "Vital North–South America and Caribbean gateway branch specializing in air cargo consolidation, perishables, and pharma cold-chain.",
		AirCargo:   "#1 US international perishables air hub • Extensive LATAM freighter network",
	},
	{
		Code: "DFW", UNLOCODE: "USDFW", ICAO: "KDFW", City: "Dallas / Fort Worth", Airport: "Dallas Fort Worth International Airport",
		Country: "United States", ISO2: "US", Currency: "USD ($)", Timezone: "America/Chicago", Region: "North America",
		PortType: "South-Central US Air & Ground Hub", Lat: 32.8998, Lon: -97.0403,
		Role:       "Central US air cargo and transcon distribution gateway branch serving Texas, electronics corridors, and cross-border Mexico trade.",
	},
	{
		Code: "YYZ", UNLOCODE: "CAYYZ", ICAO: "CYYZ", City: "Toronto", Airport: "Toronto Pearson International Airport",
		Country: "Canada", ISO2: "CA", Currency: "CAD (C$)", Timezone: "America/Toronto", Region: "North America",
		PortType: "Primary Canadian Air Gateway", Lat: 43.6777, Lon: -79.6248,
		Role:       "Canada's largest air freight and CBSA customs brokerage gateway serving Ontario and Eastern Canada.",
	},
	{
		Code: "MEX", UNLOCODE: "MXMEX", ICAO: "MMMX", City: "Mexico City", Airport: "Mexico City International Airport (AICM / NLU)",
		Country: "Mexico", ISO2: "MX", Currency: "MXN ($)", Timezone: "America/Mexico_City", Region: "North America",
		PortType: "Primary Mexico Air Gateway", Lat: 19.4361, Lon: -99.0719,
		Role:       "Key Mexican air freight and customs gateway connecting North American nearshoring and global automotive/aerospace supply chains.",
	},
	{
		Code: "LHR", UNLOCODE: "GBLHR", ICAO: "EGLL", City: "London", Airport: "London Heathrow Airport",
		Country: "United Kingdom", ISO2: "GB", Currency: "GBP (£)", Timezone: "Europe/London", Region: "EMEA",
		PortType: "Primary UK Air Cargo Gateway", Lat: 51.4700, Lon: -0.4543,
		Role:       "Principal UK gateway branch at Heathrow Cargo Centre handling trans-Atlantic and global air consolidation and CDS customs clearance.",
	},
	{
		Code: "CDG", UNLOCODE: "FRCDG", ICAO: "LFPG", City: "Paris", Airport: "Paris Charles de Gaulle Airport",
		Country: "France", ISO2: "FR", Currency: "EUR (€)", Timezone: "Europe/Paris", Region: "EMEA",
		PortType: "Principal French Air Cargo Hub", Lat: 49.0097, Lon: 2.5479,
		Role:       "Major French and European air cargo gateway branch managing luxury, aerospace, and pharmaceutical trade lanes.",
	},
	{
		Code: "DXB", UNLOCODE: "AEDXB", ICAO: "OMDB", City: "Dubai", Airport: "Dubai International & Al Maktoum (DXB / DWC)",
		Country: "United Arab Emirates", ISO2: "AE", Currency: "AED", Timezone: "Asia/Dubai", Region: "EMEA",
		PortType: "Middle East Regional HQ & Sea-Air Hub", Lat: 25.2532, Lon: 55.3657,
		Role:       "Middle East and Africa regional hub located in Dubai Airport Freezone (DAFZA), specializing in rapid Sea-Air multimodal transshipment via Jebel Ali (AEJEA) & DXB/DWC.",
		AirCargo:   "Dubai Cargo Village (DXB) & Al Maktoum (DWC) • Premier Sea-Air conversion hub (Asia to Europe in 7–10 days)",
	},
	{
		Code: "ICN", UNLOCODE: "KRICN", ICAO: "RKSI", City: "Seoul", Airport: "Incheon International Airport",
		Country: "South Korea", ISO2: "KR", Currency: "KRW (₩)", Timezone: "Asia/Seoul", Region: "APAC",
		PortType: "East Asia High-Tech Air Hub", Lat: 37.4602, Lon: 126.4407,
		Role:       "Major East Asian gateway branch handling semiconductors, displays, batteries, and trans-Pacific air freight consolidation.",
	},
	{
		Code: "NRT", UNLOCODE: "JPTYO", ICAO: "RJAA", City: "Tokyo", Airport: "Tokyo Narita International Airport",
		Country: "Japan", ISO2: "JP", Currency: "JPY (¥)", Timezone: "Asia/Tokyo", Region: "APAC",
		PortType: "Primary Japan Air Cargo Gateway", Lat: 35.7720, Lon: 140.3929,
		Role:       "Primary Japanese air freight gateway branch serving Tokyo/Kanto precision manufacturing, automotive, and semiconductor trade.",
	},
	{
		Code: "TPE", UNLOCODE: "TWTPE", ICAO: "RCTP", City: "Taipei", Airport: "Taiwan Taoyuan International Airport",
		Country: "Taiwan", ISO2: "TW", Currency: "TWD (NT$)", Timezone: "Asia/Taipei", Region: "APAC",
		PortType: "Semiconductor & Tech Air Hub", Lat: 25.0797, Lon: 121.2342,
		Role:       "Critical APAC air cargo gateway branch specializing in semiconductor foundry equipment, AI servers, and electronics exports.",
	},
	{
		Code: "SYD", UNLOCODE: "AUSYD", ICAO: "YSSY", City: "Sydney", Airport: "Sydney Kingsford Smith Airport & Port Botany",
		Country: "Australia", ISO2: "AU", Currency: "AUD (A$)", Timezone: "Australia/Sydney", Region: "APAC",
		PortType: "Primary Oceania Air & Ocean Gateway", Lat: 33.9399 * -1, Lon: 151.1753,
		Role:       "Primary Australian gateway branch near Mascot/Sydney Airport and Port Botany handling Oceania import/export consolidation.",
	},
	{
		Code: "GRU", UNLOCODE: "BRGRU", ICAO: "SBGR", City: "São Paulo", Airport: "São Paulo Guarulhos International Airport",
		Country: "Brazil", ISO2: "BR", Currency: "BRL (R$)", Timezone: "America/Sao_Paulo", Region: "South America",
		PortType: "Primary South America Air Hub", Lat: -23.4356, Lon: -46.4731,
		Role:       "South America's largest air cargo gateway branch, managing Brazilian industrial, automotive, and healthcare logistics.",
	},
	{
		Code: "RTM", UNLOCODE: "NLRTM", ICAO: "EHRD", City: "Rotterdam", Airport: "Port of Rotterdam (Maasvlakte I & II)",
		Country: "Netherlands", ISO2: "NL", Currency: "EUR (€)", Timezone: "Europe/Amsterdam", Region: "EMEA",
		PortType: "Europe's Largest Container Seaport", Lat: 51.9244, Lon: 4.4777,
		Role:       "Europe's largest deep-water container seaport with direct Rhine barge, rail, and feeder connections across Western and Central Europe.",
		OceanNote:  "Maasvlakte I & II automated deep-sea terminals • 24/7 draft for 24,000+ TEU ultra-large container vessels (ULCV)",
	},
	{
		Code: "HAM", UNLOCODE: "DEHAM", ICAO: "EDDH", City: "Hamburg", Airport: "Port of Hamburg & Hamburg Airport",
		Country: "Germany", ISO2: "DE", Currency: "EUR (€)", Timezone: "Europe/Berlin", Region: "EMEA",
		PortType: "Germany's Largest Seaport & Rail Hub", Lat: 53.5511, Lon: 9.9937,
		Role:       "Major German seaport and air gateway branch with premier European rail hinterland connectivity.",
		OceanNote:  "Container Terminals Altenwerder (CTA), Burchardkai (CTB), and Tollerort (CTT) • Europe's #1 rail port",
	},
}

// BuiltInModules returns Vex's 6 curated, toggleable domain modules.
func BuiltInModules() []models.ModuleManifest {
	geoCards := make([]models.SearchCard, 0, len(majorGateways))
	airOverlays := make([]models.CardOverlay, 0)
	oceanOverlays := make([]models.CardOverlay, 0)
	customsOverlays := make([]models.CardOverlay, 0)
	transconOverlays := make([]models.CardOverlay, 0)
	techOverlays := make([]models.CardOverlay, 0)

	for _, gw := range majorGateways {
		codeLower := strings.ToLower(gw.Code)
		entityKey := "loc:" + gw.Code
		cardID := "port-" + strings.ToLower(gw.UNLOCODE)

		geoCards = append(geoCards, models.SearchCard{
			ID:          cardID,
			EntityKey:   entityKey,
			ModuleID:    "geographies",
			Title:       fmt.Sprintf("%s — %s (%s / %s)", gw.City, gw.Airport, gw.Code, gw.UNLOCODE),
			Subtitle:    fmt.Sprintf("%s (%s) • UN/LOCODE %s • IATA %s • %s", gw.Country, gw.ISO2, gw.UNLOCODE, gw.Code, gw.Currency),
			Description: gw.Role,
			Collection:  "ports",
			Template:    models.TemplateKeyValue,
			Icon:        "globe",
			Tags: []string{
				codeLower,
				strings.ToLower(gw.UNLOCODE),
				strings.ToLower(gw.City),
				strings.ToLower(gw.Country),
				strings.ToLower(gw.ISO2),
				"iata",
				"unlocode",
				"logistics",
				"gateway",
			},
			Modifiers:   []string{"port", "iata", "locode"},
			RelatedKeys: []string{gw.Code, gw.UNLOCODE, gw.ISO2},
			Data: map[string]any{
				"iata":         gw.Code,
				"unlocode":     gw.UNLOCODE,
				"icao":         gw.ICAO,
				"country":      gw.Country,
				"country_code": gw.ISO2,
				"currency":     gw.Currency,
				"timezone":     gw.Timezone,
				"region":       gw.Region,
				"port_type":    gw.PortType,
				"branch":       "Active Gateway Branch (" + gw.Code + ")",
				"lat":          gw.Lat,
				"lon":          gw.Lon,
			},
			Actions: []models.CardAction{
				{ID: "copy-iata", Label: "Copy " + gw.Code, Shortcut: "Enter", Type: "copy", Target: gw.Code},
				{ID: "copy-locode", Label: "Copy " + gw.UNLOCODE, Type: "copy", Target: gw.UNLOCODE},
				{
					ID: "open-map", Label: "View on Map", Type: "open_url",
					Target: fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.4f,%.4f", gw.Lat, gw.Lon),
				},
			},
		})

		if gw.AirCargo != "" {
			airOverlays = append(airOverlays, models.CardOverlay{
				TargetEntityKey: entityKey,
				TargetCardID:    cardID,
				FallbackTitle:   fmt.Sprintf("%s Air Cargo Gateway (%s / %s)", gw.City, gw.Code, gw.ICAO),
				FallbackCol:     "air-freight",
				AddTags:         []string{"air", "cargo", strings.ToLower(gw.ICAO), codeLower},
				AddModifiers:    []string{"air"},
				Section: models.CardSection{
					Title: "Air Freight & Cargo Gateway",
					Badge: "Air",
					Notes: gw.AirCargo,
					Fields: map[string]any{
						"iata_icao":    gw.Code + " / " + gw.ICAO,
						"gateway_role": gw.City + " Air Consolidation & Gateway Branch",
						"uld_handling": "Main-Deck (PMC/AMJ) & Lower-Deck (AKE/LD7)",
					},
				},
			})
		}

		if gw.OceanNote != "" {
			oceanOverlays = append(oceanOverlays, models.CardOverlay{
				TargetEntityKey: entityKey,
				TargetCardID:    cardID,
				FallbackTitle:   fmt.Sprintf("Port of %s (%s)", gw.City, gw.UNLOCODE),
				FallbackCol:     "ocean-freight",
				AddTags:         []string{"ocean", "seaport", strings.ToLower(gw.UNLOCODE), codeLower},
				AddModifiers:    []string{"ocean"},
				Section: models.CardSection{
					Title: "Ocean Freight & Seaport Terminals",
					Badge: "Ocean",
					Notes: gw.OceanNote,
					Fields: map[string]any{
						"seaport_unlocode": gw.UNLOCODE,
						"fcl_lcl_gateway":  "FCL / LCL Container Consolidation",
					},
				},
			})
		}

		if gw.Customs != "" {
			customsOverlays = append(customsOverlays, models.CardOverlay{
				TargetEntityKey: entityKey,
				TargetCardID:    cardID,
				FallbackTitle:   fmt.Sprintf("%s Customs & Brokerage (%s)", gw.City, gw.Code),
				FallbackCol:     "customs",
				AddTags:         []string{"customs", "brokerage", codeLower},
				Section: models.CardSection{
					Title: "Customs & Trade Compliance",
					Badge: "Customs",
					Notes: gw.Customs,
					Fields: map[string]any{
						"customs_jurisdiction": gw.Country + " (" + gw.ISO2 + ")",
						"brokerage_branch":     "Customs Brokerage — " + gw.Code,
					},
				},
			})
		}

		if gw.Code == "AMS" || gw.Code == "FRA" || gw.Code == "ORD" || gw.Code == "LAX" || gw.Code == "DFW" {
			corridor := "North America Intermodal & FTL/LTL Corridor"
			equipment := "53' Dry Van / Reefer Road Feeder"
			if gw.Region == "EMEA" {
				corridor = "Pan-European Road Feeder Service (RFS) & Benelux/DACH Distribution"
				equipment = "13.6m Euro Tautliner / Mega Trailer (Roller-Bed ULD Capable)"
			}
			transconOverlays = append(transconOverlays, models.CardOverlay{
				TargetEntityKey: entityKey,
				TargetCardID:    cardID,
				FallbackTitle:   fmt.Sprintf("%s Transcon & Road Feeder Hub (%s)", gw.City, gw.Code),
				FallbackCol:     "transcon",
				AddTags:         []string{"transcon", "rfs", "trucking", codeLower},
				Section: models.CardSection{
					Title: "Transcon & Road Feeder Service (RFS)",
					Badge: "Transcon",
					Notes: corridor,
					Fields: map[string]any{
						"standard_equipment": equipment,
						"hub_code":           gw.Code + "-RFS",
					},
				},
			})
		}

		if gw.TechServer != "" {
			wmsHost := strings.Replace(gw.TechServer, "edi01", "wms02", 1)
			techOverlays = append(techOverlays, models.CardOverlay{
				TargetEntityKey: entityKey,
				TargetCardID:    cardID,
				FallbackTitle:   fmt.Sprintf("%s Branch IT Infrastructure (%s)", gw.City, gw.Code),
				FallbackCol:     "global-tech",
				AddTags:         []string{"it", "servers", "noc", gw.TechServer, codeLower},
				AddModifiers:    []string{"it", "srv"},
				Section: models.CardSection{
					Title: "Global Technology — Branch Infrastructure",
					Badge: "Global Tech",
					Notes: "Internal branch server hosts and network segment (visible only when Global Technology module is enabled).",
					Fields: map[string]any{
						"branch_code":      gw.Code,
						"primary_edi_host": gw.TechServer,
						"wms_node":         wmsHost,
						"branch_subnet":    gw.TechSubnet,
					},
					Actions: []models.CardAction{
						{
							ID:     "copy-edi-" + codeLower,
							Label:  "Copy " + gw.Code + " EDI Host",
							Type:   "copy",
							Target: gw.TechServer,
						},
					},
				},
			})
		}
	}

	// Pull existing curated containers, hs-codes, acronyms, and portals from LogisticsDemoCollections()
	demo := LogisticsDemoCollections()

	// Build Air Freight standalone cards (ULDs & Widebody Freighters)
	airCards := []models.SearchCard{
		{
			ID:          "uld-ake-ld3",
			EntityKey:   "uld:AKE",
			ModuleID:    "air-freight",
			Title:       "ULD AKE (LD3) — Lower-Deck Half-Width Container",
			Subtitle:    "Max Gross 1,588 kg (3,500 lbs) • 4.3 CBM (152 ft³) • Base 61.5 × 60.4 in",
			Description: "Most common widebody lower-deck air cargo container worldwide. Contoured one-side cut fits two abreast in A330, A350, B777, B787, and B747 lower holds.",
			Collection:  "air-freight",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"uld", "ake", "ld3", "air", "container", "dimensions", "weight"},
			Modifiers:   []string{"uld", "air"},
			Data: map[string]any{
				"iata_prefix":      "AKE (LD3)",
				"base_dimensions":  "61.5 × 60.4 in (156 × 153 cm)",
				"max_height":       "64 in (163 cm)",
				"max_gross_weight": "1,588 kg (3,500 lbs)",
				"tare_weight":      "~82 kg (181 lbs)",
				"internal_volume":  "4.3 m³ (152 ft³)",
				"aircraft_fit":     "A330, A350, B747, B767 (wide), B777, B787 Lower Deck",
			},
			Actions: []models.CardAction{
				{ID: "copy-ake", Label: "Copy AKE (LD3) Specs", Shortcut: "Enter", Type: "copy", Target: "AKE (LD3): Base 156x153cm (61.5x60.4in) | Height 163cm (64in) | Max Gross 1,588 kg (3,500 lbs) | 4.3 CBM"},
			},
		},
		{
			ID:          "uld-pmc-p6p",
			EntityKey:   "uld:PMC",
			ModuleID:    "air-freight",
			Title:       "ULD PMC (P6P) — 96 × 125 in Universal Air Pallet",
			Subtitle:    "Max Gross 6,804 kg (15,000 lbs Main Deck) • Up to 21.2 CBM • Base 125 × 96 in",
			Description: "Industry workhorse 10-foot flat air cargo pallet with net tie-downs. Can be built to Lower-Deck contour (up to 64 in / 163 cm) or Main-Deck Freighter contour (up to 118 in / 300 cm).",
			Collection:  "air-freight",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"uld", "pmc", "p6p", "pallet", "air", "dimensions", "freighter"},
			Modifiers:   []string{"uld", "air"},
			Data: map[string]any{
				"iata_prefix":      "PMC / P6P",
				"base_dimensions":  "125 × 96 in (317.5 × 244 cm)",
				"lower_deck_max_h": "64 in (163 cm) • ~4,626 kg (10,200 lbs)",
				"main_deck_max_h":  "96–118 in (244–300 cm) • 6,804 kg (15,000 lbs)",
				"max_volume":       "15.5–21.2 m³ (547–750 ft³)",
				"aircraft_fit":     "All widebody passenger lower decks & B747F / B777F / A330F main decks",
			},
			Actions: []models.CardAction{
				{ID: "copy-pmc", Label: "Copy PMC Specs", Shortcut: "Enter", Type: "copy", Target: "PMC (P6P) Pallet: Base 317.5x244cm (125x96in) | Lower Deck 163cm / Main Deck up to 300cm | Max Gross 6,804 kg (15,000 lbs)"},
			},
		},
		{
			ID:          "uld-ld7-p1p",
			EntityKey:   "uld:LD7",
			ModuleID:    "air-freight",
			Title:       "ULD LD7 (P1P / AAY) — 88 × 125 in Pallet / Container",
			Subtitle:    "Max Gross 6,033–6,804 kg (13,300–15,000 lbs) • 10.5–11.9 CBM • Base 125 × 88 in",
			Description: "Standard 88-inch wide 10-foot pallet or igloo container used across widebody lower holds and narrow/wide main-deck freighters.",
			Collection:  "air-freight",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"uld", "ld7", "p1p", "aay", "pag", "air", "dimensions"},
			Modifiers:   []string{"uld", "air"},
			Data: map[string]any{
				"iata_prefix":      "PAG / P1P / AAY (LD7)",
				"base_dimensions":  "125 × 88 in (317.5 × 223.5 cm)",
				"max_height":       "64 in (Lower Deck) / 82–96 in (Main Deck)",
				"max_gross_weight": "6,033–6,804 kg (13,300–15,000 lbs)",
				"internal_volume":  "9.9–11.9 m³ (350–420 ft³)",
			},
			Actions: []models.CardAction{
				{ID: "copy-ld7", Label: "Copy LD7 Specs", Shortcut: "Enter", Type: "copy", Target: "LD7 (P1P/PAG): Base 317.5x223.5cm (125x88in) | Max Gross 6,033 kg (13,300 lbs) | ~11 CBM"},
			},
		},
		{
			ID:          "uld-amj",
			EntityKey:   "uld:AMJ",
			ModuleID:    "air-freight",
			Title:       "ULD AMJ (M1) — Main-Deck Freighter Container",
			Subtitle:    "Max Gross 6,804 kg (15,000 lbs) • 16.5–17.0 CBM • 125 × 96 × 96 in",
			Description: "Rigid main-deck contoured container built on a 96 × 125 in PMC footprint for B747F, B777F, and MD-11F main decks.",
			Collection:  "air-freight",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"uld", "amj", "m1", "main deck", "freighter", "air", "dimensions"},
			Modifiers:   []string{"uld", "air"},
			Data: map[string]any{
				"iata_prefix":      "AMJ (M1)",
				"dimensions":       "125 × 96 × 96 in (317.5 × 243.8 × 243.8 cm)",
				"max_gross_weight": "6,804 kg (15,000 lbs)",
				"internal_volume":  "16.5–17.0 m³ (582–600 ft³)",
			},
			Actions: []models.CardAction{
				{ID: "copy-amj", Label: "Copy AMJ Specs", Shortcut: "Enter", Type: "copy", Target: "AMJ Main-Deck Container: 317.5x243.8x243.8cm (125x96x96in) | Max Gross 6,804 kg (15,000 lbs) | 16.8 CBM"},
			},
		},
		{
			ID:          "aircraft-b747-8f",
			EntityKey:   "aircraft:B747-8F",
			ModuleID:    "air-freight",
			Title:       "Boeing 747-8F Freighter — Cargo Doors & Payload Specs",
			Subtitle:    "Max Payload ~137.7 Metric Tons • Nose Door 104 × 98 in • Side Door 134 × 120 in",
			Description: "Flagship heavy long-haul nose-loading freighter. Swing-up nose cargo door enables loading of extra-long outsize oil/gas, aerospace, and machinery cargo.",
			Collection:  "air-freight",
			Template:    models.TemplateKeyValue,
			Icon:        "activity",
			Tags:        []string{"b747", "747-8f", "freighter", "aircraft", "nose door", "dimensions", "air"},
			Modifiers:   []string{"air", "ac"},
			Data: map[string]any{
				"max_payload":         "137,700 kg (303,700 lbs)",
				"nose_cargo_door":     "104 × 98 in (264 × 249 cm)",
				"main_deck_side_door": "134 × 120 in (340 × 305 cm)",
				"lower_deck_doors":    "110 × 66 in (280 × 167 cm)",
				"main_deck_positions": "34 × PMC (96×125 in) pallets",
				"lower_deck_capacity": "12 × PMC pallets + 2 × AKE (LD3)",
			},
			Actions: []models.CardAction{
				{ID: "copy-747", Label: "Copy B747-8F Door Specs", Shortcut: "Enter", Type: "copy", Target: "B747-8F Doors: Nose 104x98in (264x249cm) | Main Side 134x120in (340x305cm) | Lower 110x66in (280x167cm) | Payload 137.7t"},
			},
		},
		{
			ID:          "aircraft-b777f",
			EntityKey:   "aircraft:B777F",
			ModuleID:    "air-freight",
			Title:       "Boeing 777F Freighter — Cargo Doors & Payload Specs",
			Subtitle:    "Max Payload ~102 Metric Tons • Main Side Door 142 × 120 in • 27 Main-Deck Pallets",
			Description: "Long-range twin-engine widebody freighter capable of carrying full 118-inch (300 cm) high main-deck PMC pallets across trans-Pacific and trans-Atlantic lanes.",
			Collection:  "air-freight",
			Template:    models.TemplateKeyValue,
			Icon:        "activity",
			Tags:        []string{"b777f", "777f", "freighter", "aircraft", "door", "dimensions", "air"},
			Modifiers:   []string{"air", "ac"},
			Data: map[string]any{
				"max_payload":         "102,010 kg (224,900 lbs)",
				"main_deck_side_door": "142 × 120 in (361 × 305 cm)",
				"lower_fwd_door":      "105 × 67 in (269 × 170 cm)",
				"lower_aft_door":      "102 × 67 in (259 × 170 cm)",
				"main_deck_positions": "27 × PMC (96×125 in) pallets",
				"lower_deck_capacity": "10 × PMC pallets or 32 × AKE (LD3)",
			},
			Actions: []models.CardAction{
				{ID: "copy-777", Label: "Copy B777F Door Specs", Shortcut: "Enter", Type: "copy", Target: "B777F Doors: Main Side 142x120in (361x305cm) | Lower 105x67in (269x170cm) | Payload 102t (27 main-deck PMC)"},
			},
		},
	}

	// Build Ocean Freight standalone cards (ISO Containers + Ocean Portals)
	oceanCards := make([]models.SearchCard, 0, len(demo["containers"])+2)
	for _, c := range demo["containers"] {
		c.ModuleID = "ocean-freight"
		oceanCards = append(oceanCards, c)
	}
	for _, p := range demo["portals"] {
		if p.ID == "portal-ams-ocean" {
			p.ModuleID = "ocean-freight"
			p.RelatedKeys = []string{"AMS", "NLAMS", "loc:AMS"}
			oceanCards = append(oceanCards, p)
		} else if p.ID == "portal-unlocode" {
			p.ModuleID = "ocean-freight"
			oceanCards = append(oceanCards, p)
		}
	}

	// Build Customs & Trade Compliance standalone cards
	customsCards := make([]models.SearchCard, 0, len(demo["hs-codes"])+len(demo["acronyms"])+3)
	customsCards = append(customsCards, models.SearchCard{
		ID:          "compliance-ofac-sanctions",
		EntityKey:   "compliance:OFAC",
		ModuleID:    "customs",
		Title:       "OFAC Comprehensively Sanctioned Countries & Regions",
		Subtitle:    "U.S. Export / Import Embargo Reference • Cuba, Iran, North Korea, Crimea/DNR/LNR",
		Description: "U.S. Treasury OFAC comprehensive embargo programs prohibit virtually all exports, imports, transshipments, and financial services involving these destinations without a specific OFAC/BIS license. Always screen all parties against the live Consolidated Screening List (CSL / SDN).",
		Collection:  "customs",
		Template:    models.TemplateKeyValue,
		Icon:        "book-open",
		Tags:        []string{"ofac", "sanctions", "embargo", "compliance", "customs", "export", "cuba", "iran", "north korea", "sdn"},
		Modifiers:   []string{"ofac", "sanctions"},
		Data: map[string]any{
			"comprehensive_embargo": "Cuba (CU), Iran (IR), North Korea (KP), Covered Regions of Ukraine (Crimea, Donetsk DNR, Luhansk LNR)",
			"heavy_targeted_list":   "Russia (RU), Belarus (BY), Syria (SY), Venezuela (VE) — strict BIS/OFAC sectoral & SDN restrictions",
			"deemed_export_rule":    "Sharing controlled tech/data with a foreign national is treated as an export to their home country",
			"required_screening":    "OFAC SDN + BIS Denied Persons / Entity List prior to booking",
		},
		Actions: []models.CardAction{
			{ID: "open-ofac", Label: "Open Official OFAC Sanctions Search", Shortcut: "Enter", Type: "open_url", Target: "https://sanctionssearch.ofac.treas.gov/"},
			{ID: "copy-ofac", Label: "Copy Embargo Summary", Type: "copy", Target: "OFAC Comprehensive Embargoes: Cuba (CU), Iran (IR), North Korea (KP), and Covered Regions of Ukraine (Crimea, Donetsk DNR, Luhansk LNR)."},
		},
	})
	for _, hs := range demo["hs-codes"] {
		hs.ModuleID = "customs"
		customsCards = append(customsCards, hs)
	}
	for _, ac := range demo["acronyms"] {
		ac.ModuleID = "customs"
		if ac.ID == "acronym-ams" || ac.ID == "event-ams" {
			ac.RelatedKeys = []string{"AMS", "NLAMS", "loc:AMS"}
		}
		customsCards = append(customsCards, ac)
	}
	for _, p := range demo["portals"] {
		if p.ID == "portal-usitc-hts" {
			p.ModuleID = "customs"
			customsCards = append(customsCards, p)
		}
	}

	// Build Transcon standalone cards
	transconCards := []models.SearchCard{
		{
			ID:          "transcon-53-dry-van",
			EntityKey:   "trailer:53DV",
			ModuleID:    "transcon",
			Title:       "53' North America Dry Van Trailer (FTLSpecs)",
			Subtitle:    "26 Standard Pallets (48×40\") • Max Payload ~45,000 lbs (20,411 kg)",
			Description: "Standard North American over-the-road (OTR) 53-foot dry van trailer used for full truckload (FTL) and transcon linehaul.",
			Collection:  "transcon",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"53ft", "trailer", "ftl", "ltl", "transcon", "trucking", "dimensions"},
			Modifiers:   []string{"trk"},
			Data: map[string]any{
				"internal_length":  "52' 6\" (16.00 m)",
				"internal_width":   "98–101 in (2.49–2.56 m)",
				"internal_height":  "110 in (2.79 m)",
				"pallet_capacity":  "26 standard GMA pallets (48×40 in, single stack)",
				"max_gross_us_dot": "80,000 lbs total GVW (~44,000–45,000 lbs cargo payload)",
			},
			Actions: []models.CardAction{
				{ID: "copy-53dv", Label: "Copy 53' Van Specs", Shortcut: "Enter", Type: "copy", Target: "53' Dry Van: 52'6\" x 99\" x 110\" | 26 GMA Pallets (48x40) | Max Payload ~45,000 lbs"},
			},
		},
		{
			ID:          "transcon-euro-tautliner",
			EntityKey:   "trailer:136M",
			ModuleID:    "transcon",
			Title:       "13.6m European Curtain-Sider / Tautliner Trailer",
			Subtitle:    "33 Euro Pallets (120×80 cm) • Max Payload ~24,000 kg • ~90 CBM",
			Description: "Standard European cross-border road freight trailer and Air Road Feeder Service (RFS) unit.",
			Collection:  "transcon",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"tautliner", "13.6m", "euro pallet", "transcon", "rfs", "europe"},
			Modifiers:   []string{"trk"},
			Data: map[string]any{
				"internal_length": "13.60 m (44' 7\")",
				"internal_width":  "2.48 m (8' 1\")",
				"internal_height": "2.70–3.00 m (Mega Trailer)",
				"pallet_capacity": "33 Euro pallets (1200 × 800 mm)",
				"max_payload":     "24,000 kg (52,900 lbs)",
			},
			Actions: []models.CardAction{
				{ID: "copy-euro", Label: "Copy Euro Trailer Specs", Shortcut: "Enter", Type: "copy", Target: "13.6m Euro Trailer: 13.60x2.48x2.70m | 33 Euro Pallets (120x80cm) | Max Payload 24,000 kg"},
			},
		},
	}

	// Build Global Technology standalone cards (OFF by default)
	techCards := []models.SearchCard{
		{
			ID:          "tech-noc-portal",
			EntityKey:   "tech:NOC",
			ModuleID:    "global-tech",
			Title:       "Global Technology NOC & EDI Status Portal",
			Subtitle:    "Internal IT Operations • Branch Server, WMS & EDI Gateway Health",
			Description: "Launch the Global Technology Network Operations Center (NOC) dashboard to view branch EDI queue depth, WMS node latency, and customs broker gateway uptime.",
			Collection:  "global-tech",
			Template:    models.TemplateAppShortcut,
			Icon:        "terminal",
			Tags:        []string{"it", "noc", "edi", "wms", "servers", "global-tech"},
			Modifiers:   []string{"noc", "it"},
			Data: map[string]any{
				"url":     "https://noc.internal.corp.internal/branches",
				"on_call": "Global Tech SRE / EDI Ops",
			},
			Actions: []models.CardAction{
				{ID: "open-noc", Label: "Open NOC Dashboard", Shortcut: "Enter", Type: "open_url", Target: "https://noc.internal.corp.internal/branches/{{arg1}}"},
			},
		},
	}

	return []models.ModuleManifest{
		{
			ID:             "geographies",
			Name:           "Geographies & Global Gateways",
			Category:       "Core & Locations",
			Description:    "23 major global air & ocean gateways with branch presence, live local time, weather, UN/LOCODEs, IATA codes, and currencies.",
			Icon:           "globe",
			Version:        "1.0.0",
			DefaultEnabled: true,
			Cards:          geoCards,
		},
		{
			ID:             "air-freight",
			Name:           "Air Freight, ULDs & Freighters",
			Category:       "Product & Operations",
			Description:    "ULD container & pallet dimensions (AKE/LD3, PMC, LD7, AMJ), B747-8F & B777F cargo doors, plus air cargo terminal overlays on gateway cards.",
			Icon:           "plane",
			Version:        "1.0.0",
			DefaultEnabled: true,
			Cards:          airCards,
			Overlays:       airOverlays,
		},
		{
			ID:             "ocean-freight",
			Name:           "Ocean Freight & ISO Containers",
			Category:       "Product & Operations",
			Description:    "ISO container specs (20GP, 40GP, 40HC, 40RF Reefer), AMS Ocean Shipment Portal, and seaport terminal overlays.",
			Icon:           "box",
			Version:        "1.0.0",
			DefaultEnabled: true,
			Cards:          oceanCards,
			Overlays:       oceanOverlays,
		},
		{
			ID:             "customs",
			Name:           "Customs, HS Codes & Compliance",
			Category:       "Compliance & Trade",
			Description:    "OFAC comprehensively sanctioned countries, Harmonized Schedule (HS) codes, Incoterms 2020, AMS/ISF filings, and local customs overlays.",
			Icon:           "shield",
			Version:        "1.0.0",
			DefaultEnabled: true,
			Cards:          customsCards,
			Overlays:       customsOverlays,
		},
		{
			ID:             "transcon",
			Name:           "Transcon & Road Feeder (Ground)",
			Category:       "Product & Operations",
			Description:    "North American 53' Dry Van & European 13.6m Tautliner trailer specs plus Road Feeder Service (RFS) hub overlays.",
			Icon:           "truck",
			Version:        "1.0.0",
			DefaultEnabled: false,
			Cards:          transconCards,
			Overlays:       transconOverlays,
		},
		{
			ID:             "global-tech",
			Name:           "Global Technology (Branch IT & Servers)",
			Category:       "Technology",
			Description:    "Enriches gateway cards (AMS, SEA, FRA, HKG, PVG, LAX, ORD, SIN) with internal EDI/WMS server hostnames and VLAN subnets. Off by default for non-tech users.",
			Icon:           "terminal",
			Version:        "1.0.0",
			DefaultEnabled: false,
			Cards:          techCards,
			Overlays:       techOverlays,
		},
		BuildGitHubCatalogModule(nil),
	}
}

func defaultModuleProvenance(mod models.ModuleManifest) *models.Provenance {
	var prov *models.Provenance
	switch mod.ID {
	case "geographies":
		prov = &models.Provenance{
			SourceName: "UNECE UN/LOCODE, IATA & Global Gateway Directory",
			SourceURL:  "https://unece.org/trade/cefact/unlocode-code-list-country-and-territory",
			VerifiedAt: "2026-11-21",
			Authority:  "official",
		}
	case "air-freight":
		prov = &models.Provenance{
			SourceName: "IATA ULD Regulations & Boeing Freighter Airplane Characteristics",
			SourceURL:  "https://www.iata.org/en/programs/cargo/uld/",
			VerifiedAt: "2026-11-21",
			Authority:  "official",
		}
	case "ocean-freight":
		prov = &models.Provenance{
			SourceName: "ISO 668 / BIC Container Specifications",
			SourceURL:  "https://www.bic-code.org/",
			VerifiedAt: "2026-11-21",
			Authority:  "official",
		}
	case "customs":
		prov = &models.Provenance{
			SourceName: "U.S. Treasury OFAC, USITC HTS & U.S. CBP",
			SourceURL:  "https://sanctionssearch.ofac.treas.gov/",
			VerifiedAt: "2026-11-21",
			Authority:  "official",
		}
	case "transcon":
		prov = &models.Provenance{
			SourceName: "U.S. DOT FMCSA & EU Road Freight Standards",
			VerifiedAt: "2026-11-21",
			Authority:  "official",
		}
	case "global-tech":
		prov = &models.Provenance{
			SourceName: "Global Technology CMDB & NOC Inventory",
			SourceURL:  "https://noc.internal.corp.internal/branches",
			VerifiedAt: "2026-11-21",
			Authority:  "internal-curated",
		}
	case "github-catalog":
		prov = &models.Provenance{
			SourceName: "GitHub Engineering Catalog (gt-*)",
			SourceURL:  "https://github.internal.corp.internal/gt",
			VerifiedAt: "2026-11-21",
			Authority:  "automated-sync",
		}
	default:
		prov = &models.Provenance{
			SourceName: mod.Name,
			VerifiedAt: "2026-11-21",
			Authority:  "internal-curated",
		}
	}
	if mod.LocalOnly {
		prov.LocalOnly = true
	}
	return prov
}

// DefaultEnabledModuleIDs returns the IDs of modules that are enabled by default.
func DefaultEnabledModuleIDs() []string {
	var out []string
	for _, m := range BuiltInModules() {
		if m.DefaultEnabled {
			out = append(out, m.ID)
		}
	}
	return out
}

// LoadAllModules combines built-in modules with any user-imported *.json module manifests in modulesDir.
func LoadAllModules(modulesDir string) ([]models.ModuleManifest, map[string]bool, error) {
	builtins := BuiltInModules()
	byID := make(map[string]models.ModuleManifest, len(builtins))
	isBuiltIn := make(map[string]bool, len(builtins))
	order := make([]string, 0, len(builtins))

	for _, m := range builtins {
		byID[m.ID] = m
		isBuiltIn[m.ID] = true
		order = append(order, m.ID)
	}

	if strings.TrimSpace(modulesDir) != "" {
		entries, err := os.ReadDir(modulesDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
					continue
				}
				raw, readErr := os.ReadFile(filepath.Join(modulesDir, e.Name()))
				if readErr != nil {
					continue
				}
				var custom models.ModuleManifest
				if jsonErr := json.Unmarshal(raw, &custom); jsonErr != nil {
					continue
				}
				custom.ID = models.Slugify(custom.ID)
				if custom.ID == "" || strings.TrimSpace(custom.Name) == "" {
					continue
				}
				if _, exists := byID[custom.ID]; !exists {
					order = append(order, custom.ID)
				}
				byID[custom.ID] = custom
			}
		}
	}

	out := make([]models.ModuleManifest, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, isBuiltIn, nil
}

// SaveCustomModule validates and persists a shareable ModuleManifest JSON file to modulesDir.
func SaveCustomModule(modulesDir string, mod models.ModuleManifest) (models.ModuleManifest, error) {
	mod.Name = strings.TrimSpace(mod.Name)
	if mod.Name == "" {
		return mod, errors.New("module name is required")
	}
	mod.ID = models.Slugify(mod.ID)
	if mod.ID == "" {
		mod.ID = models.Slugify(mod.Name)
	}
	if mod.Category == "" {
		mod.Category = "Custom Shared"
	}
	if mod.Version == "" {
		mod.Version = "1.0.0"
	}
	for i := range mod.Cards {
		mod.Cards[i].ModuleID = mod.ID
		if err := models.ValidateAndNormalizeCard(&mod.Cards[i], mod.ID); err != nil {
			return mod, fmt.Errorf("card %d: %w", i, err)
		}
	}
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		return mod, err
	}
	raw, err := json.MarshalIndent(mod, "", "  ")
	if err != nil {
		return mod, err
	}
	raw = append(raw, '\n')
	dest := filepath.Join(modulesDir, mod.ID+".json")
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return mod, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return mod, err
	}
	return mod, nil
}

// CompileActiveModules merges all enabled modules' standalone cards and composable overlays.
// If an overlay targets an entity whose base module is disabled, it automatically promotes
// the overlay into a standalone card so every module works both standalone and combined.
func CompileActiveModules(all []models.ModuleManifest, enabledIDs []string) []models.SearchCard {
	enabledSet := make(map[string]bool)
	if enabledIDs == nil {
		for _, m := range all {
			if m.DefaultEnabled {
				enabledSet[m.ID] = true
			}
		}
	} else {
		for _, id := range enabledIDs {
			enabledSet[strings.TrimSpace(id)] = true
		}
	}

	cards := make([]models.SearchCard, 0, 64)
	byID := make(map[string]int)
	byEntity := make(map[string]int)

	// Pass 1: Collect standalone base cards from enabled modules
	for _, mod := range all {
		if !enabledSet[mod.ID] {
			continue
		}
		modProv := defaultModuleProvenance(mod)
		for _, c := range mod.Cards {
			cCopy := cloneCard(c)
			cCopy.ModuleID = mod.ID
			if mod.LocalOnly {
				cCopy.LocalOnly = true
			}
			if cCopy.Provenance == nil {
				cCopy.Provenance = modProv
			} else if mod.LocalOnly && !cCopy.Provenance.LocalOnly {
				provCopy := *cCopy.Provenance
				provCopy.LocalOnly = true
				cCopy.Provenance = &provCopy
			}
			if err := models.ValidateAndNormalizeCard(&cCopy, mod.ID); err != nil {
				continue
			}
			idx := len(cards)
			cards = append(cards, cCopy)
			byID[cCopy.ID] = idx
			if cCopy.EntityKey != "" {
				byEntity[strings.ToLower(cCopy.EntityKey)] = idx
			}
		}
	}

	// Pass 2: Apply overlays from enabled modules (enriching existing cards or promoting standalone)
	for _, mod := range all {
		if !enabledSet[mod.ID] {
			continue
		}
		modProv := defaultModuleProvenance(mod)
		for _, ov := range mod.Overlays {
			sec := ov.Section
			sec.ModuleID = mod.ID
			sec.ModuleName = mod.Name
			if mod.LocalOnly {
				sec.LocalOnly = true
			}
			if sec.Badge == "" {
				sec.Badge = mod.Name
			}
			if sec.Provenance == nil {
				sec.Provenance = modProv
			} else if mod.LocalOnly && !sec.Provenance.LocalOnly {
				provCopy := *sec.Provenance
				provCopy.LocalOnly = true
				sec.Provenance = &provCopy
			}

			targetIdx := -1
			if ov.TargetEntityKey != "" {
				if idx, ok := byEntity[strings.ToLower(ov.TargetEntityKey)]; ok {
					targetIdx = idx
				}
			}
			if targetIdx < 0 && ov.TargetCardID != "" {
				if idx, ok := byID[ov.TargetCardID]; ok {
					targetIdx = idx
				}
			}

			if targetIdx >= 0 {
				// Enrich existing base card with this module's section, tags, modifiers, and related keys
				dst := &cards[targetIdx]
				dst.Sections = append(dst.Sections, sec)
				dst.Tags = appendUniqueStrings(dst.Tags, ov.AddTags...)
				dst.Modifiers = appendUniqueStrings(dst.Modifiers, ov.AddModifiers...)
				dst.RelatedKeys = appendUniqueStrings(dst.RelatedKeys, ov.AddRelatedKeys...)
			} else {
				// Base card's module is turned off -> promote overlay to a standalone card!
				fallbackCol := ov.FallbackCol
				if fallbackCol == "" {
					fallbackCol = mod.ID
				}
				fallbackTitle := ov.FallbackTitle
				if fallbackTitle == "" {
					fallbackTitle = fmt.Sprintf("%s — %s", ov.TargetEntityKey, sec.Title)
				}
				synthID := ov.TargetCardID
				if synthID == "" {
					synthID = models.Slugify(mod.ID + "-" + ov.TargetEntityKey)
				}
				synth := models.SearchCard{
					ID:          synthID,
					EntityKey:   ov.TargetEntityKey,
					ModuleID:    mod.ID,
					Title:       fallbackTitle,
					Subtitle:    sec.Title + " • " + mod.Name,
					Description: sec.Notes,
					Collection:  fallbackCol,
					Template:    models.TemplateKeyValue,
					Icon:        mod.Icon,
					Tags:        appendUniqueStrings(nil, ov.AddTags...),
					Modifiers:   appendUniqueStrings(nil, ov.AddModifiers...),
					RelatedKeys: appendUniqueStrings(nil, ov.AddRelatedKeys...),
					Sections:    []models.CardSection{sec},
					Actions:     sec.Actions,
					Provenance:  sec.Provenance,
					LocalOnly:   mod.LocalOnly || sec.LocalOnly,
				}
				if err := models.ValidateAndNormalizeCard(&synth, fallbackCol); err == nil {
					idx := len(cards)
					cards = append(cards, synth)
					byID[synth.ID] = idx
					if synth.EntityKey != "" {
						byEntity[strings.ToLower(synth.EntityKey)] = idx
					}
				}
			}
		}
	}

	return cards
}

// SyncModules compiles enabled modules and atomically updates the SQLite store.
func SyncModules(ctx context.Context, store *Store, modulesDir string, enabledIDs []string) ([]models.ModuleStatus, error) {
	all, isBuiltIn, err := LoadAllModules(modulesDir)
	if err != nil {
		return nil, err
	}
	compiled := CompileActiveModules(all, enabledIDs)
	if err := store.ReplaceSourceFile(ctx, CompiledModulesSource, compiled); err != nil {
		return nil, err
	}

	enabledSet := make(map[string]bool)
	if enabledIDs == nil {
		for _, m := range all {
			if m.DefaultEnabled {
				enabledSet[m.ID] = true
			}
		}
	} else {
		for _, id := range enabledIDs {
			enabledSet[strings.TrimSpace(id)] = true
		}
	}

	statuses := make([]models.ModuleStatus, 0, len(all))
	for _, m := range all {
		statuses = append(statuses, models.ModuleStatus{
			ID:             m.ID,
			Name:           m.Name,
			Category:       m.Category,
			Description:    m.Description,
			Icon:           m.Icon,
			Version:        m.Version,
			Enabled:        enabledSet[m.ID],
			DefaultEnabled: m.DefaultEnabled,
			BuiltIn:        isBuiltIn[m.ID],
			LocalOnly:      m.LocalOnly,
			CardCount:      len(m.Cards),
			OverlayCount:   len(m.Overlays),
		})
	}
	return statuses, nil
}

func cloneCard(c models.SearchCard) models.SearchCard {
	out := c
	if len(c.Tags) > 0 {
		out.Tags = append([]string(nil), c.Tags...)
	}
	if len(c.Modifiers) > 0 {
		out.Modifiers = append([]string(nil), c.Modifiers...)
	}
	if len(c.RelatedKeys) > 0 {
		out.RelatedKeys = append([]string(nil), c.RelatedKeys...)
	}
	if len(c.Actions) > 0 {
		out.Actions = append([]models.CardAction(nil), c.Actions...)
	}
	if len(c.Sections) > 0 {
		out.Sections = append([]models.CardSection(nil), c.Sections...)
	}
	if len(c.Data) > 0 {
		out.Data = make(map[string]any, len(c.Data))
		for k, v := range c.Data {
			out.Data[k] = v
		}
	}
	return out
}

func appendUniqueStrings(dst []string, items ...string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, s := range dst {
		seen[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	for _, item := range items {
		clean := strings.TrimSpace(item)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			dst = append(dst, clean)
		}
	}
	sort.Strings(dst)
	return dst
}
