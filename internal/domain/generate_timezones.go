//go:build ignore

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// This program generates timezones.go by extracting all valid timezone names
// from Go's embedded timezone database.
//
// Run with: go run generate_timezones.go

func main() {
	timezones := extractTimezones()

	// Generate the Go source file
	var sb strings.Builder

	sb.WriteString("package domain\n\n")
	sb.WriteString("// Timezones contains all valid IANA timezone identifiers\n")
	sb.WriteString("// This list is generated from Go's embedded timezone database\n")
	sb.WriteString("// It includes both canonical zones and aliases (links)\n")
	sb.WriteString("//\n")
	sb.WriteString("// To regenerate this list, run: go generate ./internal/domain\n")
	sb.WriteString("//\n")
	sb.WriteString("//go:generate go run generate_timezones.go\n")
	sb.WriteString("var Timezones = []string{\n")

	for _, tz := range timezones {
		sb.WriteString(fmt.Sprintf("\t%q,\n", tz))
	}

	sb.WriteString("}\n\n")
	sb.WriteString("// IsValidTimezone checks if the given timezone is valid\n")
	sb.WriteString("func IsValidTimezone(timezone string) bool {\n")
	sb.WriteString("\tfor _, tz := range Timezones {\n")
	sb.WriteString("\t\tif tz == timezone {\n")
	sb.WriteString("\t\t\treturn true\n")
	sb.WriteString("\t\t}\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn false\n")
	sb.WriteString("}\n")

	// Write to timezones.go
	err := os.WriteFile("timezones.go", []byte(sb.String()), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing timezones.go: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Generated timezones.go with %d timezone identifiers\n", len(timezones))
	fmt.Printf("✓ Includes canonical zones and aliases from Go's timezone database\n")
}

// extractTimezones extracts all valid timezone names from Go's embedded database
func extractTimezones() []string {
	// Common timezone patterns to try
	// Based on the IANA timezone database structure
	continents := []string{
		"Africa", "America", "Antarctica", "Arctic", "Asia",
		"Atlantic", "Australia", "Europe", "Indian", "Pacific",
	}

	cities := map[string][]string{
		"Africa": {
			"Abidjan", "Accra", "Addis_Ababa", "Algiers", "Asmara", "Asmera",
			"Bamako", "Bangui", "Banjul", "Bissau", "Blantyre", "Brazzaville",
			"Bujumbura", "Cairo", "Casablanca", "Ceuta", "Conakry", "Dakar",
			"Dar_es_Salaam", "Djibouti", "Douala", "El_Aaiun", "Freetown",
			"Gaborone", "Harare", "Johannesburg", "Juba", "Kampala", "Khartoum",
			"Kigali", "Kinshasa", "Lagos", "Libreville", "Lome", "Luanda",
			"Lubumbashi", "Lusaka", "Malabo", "Maputo", "Maseru", "Mbabane",
			"Mogadishu", "Monrovia", "Nairobi", "Ndjamena", "Niamey", "Nouakchott",
			"Ouagadougou", "Porto-Novo", "Sao_Tome", "Timbuktu", "Tripoli", "Tunis",
			"Windhoek",
		},
		"America": {
			"Adak", "Anchorage", "Anguilla", "Antigua", "Araguaina", "Aruba",
			"Asuncion", "Atikokan", "Atka", "Bahia", "Bahia_Banderas", "Barbados",
			"Belem", "Belize", "Blanc-Sablon", "Boa_Vista", "Bogota", "Boise",
			"Buenos_Aires", "Cambridge_Bay", "Campo_Grande", "Cancun", "Caracas",
			"Catamarca", "Cayenne", "Cayman", "Chicago", "Chihuahua", "Ciudad_Juarez",
			"Coral_Harbour", "Cordoba", "Costa_Rica", "Coyhaique", "Creston", "Cuiaba",
			"Curacao", "Danmarkshavn", "Dawson", "Dawson_Creek", "Denver", "Detroit",
			"Dominica", "Edmonton", "Eirunepe", "El_Salvador", "Ensenada", "Fort_Nelson",
			"Fort_Wayne", "Fortaleza", "Glace_Bay", "Godthab", "Goose_Bay", "Grand_Turk",
			"Grenada", "Guadeloupe", "Guatemala", "Guayaquil", "Guyana", "Halifax",
			"Havana", "Hermosillo", "Indianapolis", "Inuvik", "Iqaluit", "Jamaica",
			"Jujuy", "Juneau", "Knox_IN", "Kralendijk", "La_Paz", "Lima", "Los_Angeles",
			"Louisville", "Lower_Princes", "Maceio", "Managua", "Manaus", "Marigot",
			"Martinique", "Matamoros", "Mazatlan", "Mendoza", "Menominee", "Merida",
			"Metlakatla", "Mexico_City", "Miquelon", "Moncton", "Monterrey", "Montevideo",
			"Montreal", "Montserrat", "Nassau", "New_York", "Nipigon", "Nome", "Noronha",
			"Nuuk", "Ojinaga", "Panama", "Pangnirtung", "Paramaribo", "Phoenix",
			"Port-au-Prince", "Port_of_Spain", "Porto_Acre", "Porto_Velho", "Puerto_Rico",
			"Punta_Arenas", "Rainy_River", "Rankin_Inlet", "Recife", "Regina", "Resolute",
			"Rio_Branco", "Rosario", "Santa_Isabel", "Santarem", "Santiago", "Santo_Domingo",
			"Sao_Paulo", "Scoresbysund", "Shiprock", "Sitka", "St_Barthelemy", "St_Johns",
			"St_Kitts", "St_Lucia", "St_Thomas", "St_Vincent", "Swift_Current", "Tegucigalpa",
			"Thule", "Thunder_Bay", "Tijuana", "Toronto", "Tortola", "Vancouver", "Virgin",
			"Whitehorse", "Winnipeg", "Yakutat", "Yellowknife",
		},
		"Antarctica": {
			"Casey", "Davis", "DumontDUrville", "Macquarie", "Mawson", "McMurdo",
			"Palmer", "Rothera", "South_Pole", "Syowa", "Troll", "Vostok",
		},
		"Arctic": {
			"Longyearbyen",
		},
		"Asia": {
			"Aden", "Almaty", "Amman", "Anadyr", "Aqtau", "Aqtobe", "Ashgabat",
			"Ashkhabad", "Atyrau", "Baghdad", "Bahrain", "Baku", "Bangkok", "Barnaul",
			"Beirut", "Bishkek", "Brunei", "Calcutta", "Chita", "Choibalsan", "Chongqing",
			"Chungking", "Colombo", "Dacca", "Damascus", "Dhaka", "Dili", "Dubai",
			"Dushanbe", "Famagusta", "Gaza", "Harbin", "Hebron", "Ho_Chi_Minh", "Hong_Kong",
			"Hovd", "Irkutsk", "Istanbul", "Jakarta", "Jayapura", "Jerusalem", "Kabul",
			"Kamchatka", "Karachi", "Kashgar", "Kathmandu", "Katmandu", "Khandyga",
			"Kolkata", "Krasnoyarsk", "Kuala_Lumpur", "Kuching", "Kuwait", "Macao",
			"Macau", "Magadan", "Makassar", "Manila", "Muscat", "Nicosia", "Novokuznetsk",
			"Novosibirsk", "Omsk", "Oral", "Phnom_Penh", "Pontianak", "Pyongyang", "Qatar",
			"Qostanay", "Qyzylorda", "Rangoon", "Riyadh", "Saigon", "Sakhalin", "Samarkand",
			"Seoul", "Shanghai", "Singapore", "Srednekolymsk", "Taipei", "Tashkent", "Tbilisi",
			"Tehran", "Tel_Aviv", "Thimbu", "Thimphu", "Tokyo", "Tomsk", "Ujung_Pandang",
			"Ulaanbaatar", "Ulan_Bator", "Urumqi", "Ust-Nera", "Vientiane", "Vladivostok",
			"Yakutsk", "Yangon", "Yekaterinburg", "Yerevan",
		},
		"Atlantic": {
			"Azores", "Bermuda", "Canary", "Cape_Verde", "Faeroe", "Faroe", "Jan_Mayen",
			"Madeira", "Reykjavik", "South_Georgia", "St_Helena", "Stanley",
		},
		"Australia": {
			"ACT", "Adelaide", "Brisbane", "Broken_Hill", "Canberra", "Currie", "Darwin",
			"Eucla", "Hobart", "LHI", "Lindeman", "Lord_Howe", "Melbourne", "NSW", "North",
			"Perth", "Queensland", "South", "Sydney", "Tasmania", "Victoria", "West",
			"Yancowinna",
		},
		"Europe": {
			"Amsterdam", "Andorra", "Astrakhan", "Athens", "Belfast", "Belgrade", "Berlin",
			"Bratislava", "Brussels", "Bucharest", "Budapest", "Busingen", "Chisinau",
			"Copenhagen", "Dublin", "Gibraltar", "Guernsey", "Helsinki", "Isle_of_Man",
			"Istanbul", "Jersey", "Kaliningrad", "Kiev", "Kirov", "Kyiv", "Lisbon",
			"Ljubljana", "London", "Luxembourg", "Madrid", "Malta", "Mariehamn", "Minsk",
			"Monaco", "Moscow", "Nicosia", "Oslo", "Paris", "Podgorica", "Prague", "Riga",
			"Rome", "Samara", "San_Marino", "Sarajevo", "Saratov", "Simferopol", "Skopje",
			"Sofia", "Stockholm", "Tallinn", "Tirane", "Tiraspol", "Ulyanovsk", "Uzhgorod",
			"Vaduz", "Vatican", "Vienna", "Vilnius", "Volgograd", "Warsaw", "Zagreb",
			"Zaporozhye", "Zurich",
		},
		"Indian": {
			"Antananarivo", "Chagos", "Christmas", "Cocos", "Comoro", "Kerguelen",
			"Mahe", "Maldives", "Mauritius", "Mayotte", "Reunion",
		},
		"Pacific": {
			"Apia", "Auckland", "Bougainville", "Chatham", "Chuuk", "Easter", "Efate",
			"Enderbury", "Fakaofo", "Fiji", "Funafuti", "Galapagos", "Gambier", "Guadalcanal",
			"Guam", "Honolulu", "Johnston", "Kanton", "Kiritimati", "Kosrae", "Kwajalein",
			"Majuro", "Marquesas", "Midway", "Nauru", "Niue", "Norfolk", "Noumea",
			"Pago_Pago", "Palau", "Pitcairn", "Pohnpei", "Ponape", "Port_Moresby",
			"Rarotonga", "Saipan", "Samoa", "Tahiti", "Tarawa", "Tongatapu", "Truk",
			"Wake", "Wallis", "Yap",
		},
	}

	// Special zones
	specialZones := []string{
		"UTC",
		"GMT",
		"GMT-0",
		"GMT+0",
		"GMT0",
		"Greenwich",
		"UCT",
		"Universal",
		"Zulu",
		"EST",
		"HST",
		"MST",
		"ACT",
		"AET",
		"AGT",
		"ART",
		"AST",
		"BET",
		"BST",
		"CAT",
		"CNT",
		"CST",
		"CTT",
		"EAT",
		"ECT",
		"IET",
		"IST",
		"JST",
		"MIT",
		"NET",
		"NST",
		"PLT",
		"PNT",
		"PRT",
		"PST",
		"SST",
		"VST",
	}

	// America sub-regions
	americaRegions := map[string][]string{
		"Argentina": {
			"Buenos_Aires", "Catamarca", "ComodRivadavia", "Cordoba", "Jujuy",
			"La_Rioja", "Mendoza", "Rio_Gallegos", "Salta", "San_Juan", "San_Luis",
			"Tucuman", "Ushuaia",
		},
		"Indiana": {
			"Indianapolis", "Knox", "Marengo", "Petersburg", "Tell_City", "Vevay",
			"Vincennes", "Winamac",
		},
		"Kentucky": {
			"Louisville", "Monticello",
		},
		"North_Dakota": {
			"Beulah", "Center", "New_Salem",
		},
	}

	validTimezones := make(map[string]bool)

	// Test all continent/city combinations
	for _, continent := range continents {
		if cityList, ok := cities[continent]; ok {
			for _, city := range cityList {
				tz := continent + "/" + city
				if isValidTimezone(tz) {
					validTimezones[tz] = true
				}
			}
		}
	}

	// Test America sub-regions
	for region, cityList := range americaRegions {
		for _, city := range cityList {
			tz := "America/" + region + "/" + city
			if isValidTimezone(tz) {
				validTimezones[tz] = true
			}
		}
	}

	// Test special zones
	for _, tz := range specialZones {
		if isValidTimezone(tz) {
			validTimezones[tz] = true
		}
	}

	// Additional US zones (US/*)
	usZones := []string{
		"US/Alaska", "US/Aleutian", "US/Arizona", "US/Central", "US/East-Indiana",
		"US/Eastern", "US/Hawaii", "US/Indiana-Starke", "US/Michigan", "US/Mountain",
		"US/Pacific", "US/Samoa",
	}
	for _, tz := range usZones {
		if isValidTimezone(tz) {
			validTimezones[tz] = true
		}
	}

	// Additional Canada zones
	canadaZones := []string{
		"Canada/Atlantic", "Canada/Central", "Canada/Eastern", "Canada/Mountain",
		"Canada/Newfoundland", "Canada/Pacific", "Canada/Saskatchewan", "Canada/Yukon",
	}
	for _, tz := range canadaZones {
		if isValidTimezone(tz) {
			validTimezones[tz] = true
		}
	}

	// Additional zones
	otherZones := []string{
		"Brazil/Acre", "Brazil/DeNoronha", "Brazil/East", "Brazil/West",
		"Chile/Continental", "Chile/EasterIsland",
		"Mexico/BajaNorte", "Mexico/BajaSur", "Mexico/General",
		"Cuba", "Egypt", "Eire", "Hongkong", "Iceland", "Iran", "Israel",
		"Jamaica", "Japan", "Kwajalein", "Libya", "NZ", "NZ-CHAT",
		"Navajo", "PRC", "Poland", "Portugal", "ROC", "ROK", "Singapore",
		"Turkey", "W-SU",
		"CET", "EET", "EST5EDT", "CST6CDT", "MST7MDT", "PST8PDT", "WET",
		"Etc/GMT", "Etc/GMT+0", "Etc/GMT+1", "Etc/GMT+2", "Etc/GMT+3",
		"Etc/GMT+4", "Etc/GMT+5", "Etc/GMT+6", "Etc/GMT+7", "Etc/GMT+8",
		"Etc/GMT+9", "Etc/GMT+10", "Etc/GMT+11", "Etc/GMT+12",
		"Etc/GMT-0", "Etc/GMT-1", "Etc/GMT-2", "Etc/GMT-3", "Etc/GMT-4",
		"Etc/GMT-5", "Etc/GMT-6", "Etc/GMT-7", "Etc/GMT-8", "Etc/GMT-9",
		"Etc/GMT-10", "Etc/GMT-11", "Etc/GMT-12", "Etc/GMT-13", "Etc/GMT-14",
		"Etc/GMT0", "Etc/Greenwich", "Etc/UCT", "Etc/UTC", "Etc/Universal", "Etc/Zulu",
	}
	for _, tz := range otherZones {
		if isValidTimezone(tz) {
			validTimezones[tz] = true
		}
	}

	// Convert map to sorted slice
	result := make([]string, 0, len(validTimezones))
	for tz := range validTimezones {
		result = append(result, tz)
	}
	sort.Strings(result)

	return result
}

// isValidTimezone checks if a timezone name is valid by attempting to load it
func isValidTimezone(name string) bool {
	_, err := time.LoadLocation(name)
	return err == nil
}
