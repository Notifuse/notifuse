// ISO 3166-1 lookup tables for the country dimension.
//
// GeoIP records countries as alpha-2 codes, ECharts matches map features by
// their alpha-3 feature id, and both the map tooltip and the country tables
// need a readable name, so all three representations live here. Names are the
// CLDR English short forms, which read better than the GeoJSON's formal ones
// ("United States" rather than "United States of America").

export const ISO2_TO_ISO3: Record<string, string> = {
  AD: 'AND', AE: 'ARE', AF: 'AFG', AG: 'ATG', AI: 'AIA', AL: 'ALB', AM: 'ARM', AO: 'AGO',
  AQ: 'ATA', AR: 'ARG', AS: 'ASM', AT: 'AUT', AU: 'AUS', AW: 'ABW', AX: 'ALA', AZ: 'AZE',
  BA: 'BIH', BB: 'BRB', BD: 'BGD', BE: 'BEL', BF: 'BFA', BG: 'BGR', BH: 'BHR', BI: 'BDI',
  BJ: 'BEN', BL: 'BLM', BM: 'BMU', BN: 'BRN', BO: 'BOL', BQ: 'BES', BR: 'BRA', BS: 'BHS',
  BT: 'BTN', BV: 'BVT', BW: 'BWA', BY: 'BLR', BZ: 'BLZ', CA: 'CAN', CC: 'CCK', CD: 'COD',
  CF: 'CAF', CG: 'COG', CH: 'CHE', CI: 'CIV', CK: 'COK', CL: 'CHL', CM: 'CMR', CN: 'CHN',
  CO: 'COL', CR: 'CRI', CU: 'CUB', CV: 'CPV', CW: 'CUW', CX: 'CXR', CY: 'CYP', CZ: 'CZE',
  DE: 'DEU', DJ: 'DJI', DK: 'DNK', DM: 'DMA', DO: 'DOM', DZ: 'DZA', EC: 'ECU', EE: 'EST',
  EG: 'EGY', EH: 'ESH', ER: 'ERI', ES: 'ESP', ET: 'ETH', FI: 'FIN', FJ: 'FJI', FK: 'FLK',
  FM: 'FSM', FO: 'FRO', FR: 'FRA', GA: 'GAB', GB: 'GBR', GD: 'GRD', GE: 'GEO', GF: 'GUF',
  GG: 'GGY', GH: 'GHA', GI: 'GIB', GL: 'GRL', GM: 'GMB', GN: 'GIN', GP: 'GLP', GQ: 'GNQ',
  GR: 'GRC', GS: 'SGS', GT: 'GTM', GU: 'GUM', GW: 'GNB', GY: 'GUY', HK: 'HKG', HM: 'HMD',
  HN: 'HND', HR: 'HRV', HT: 'HTI', HU: 'HUN', ID: 'IDN', IE: 'IRL', IL: 'ISR', IM: 'IMN',
  IN: 'IND', IO: 'IOT', IQ: 'IRQ', IR: 'IRN', IS: 'ISL', IT: 'ITA', JE: 'JEY', JM: 'JAM',
  JO: 'JOR', JP: 'JPN', KE: 'KEN', KG: 'KGZ', KH: 'KHM', KI: 'KIR', KM: 'COM', KN: 'KNA',
  KP: 'PRK', KR: 'KOR', KW: 'KWT', KY: 'CYM', KZ: 'KAZ', LA: 'LAO', LB: 'LBN', LC: 'LCA',
  LI: 'LIE', LK: 'LKA', LR: 'LBR', LS: 'LSO', LT: 'LTU', LU: 'LUX', LV: 'LVA', LY: 'LBY',
  MA: 'MAR', MC: 'MCO', MD: 'MDA', ME: 'MNE', MF: 'MAF', MG: 'MDG', MH: 'MHL', MK: 'MKD',
  ML: 'MLI', MM: 'MMR', MN: 'MNG', MO: 'MAC', MP: 'MNP', MQ: 'MTQ', MR: 'MRT', MS: 'MSR',
  MT: 'MLT', MU: 'MUS', MV: 'MDV', MW: 'MWI', MX: 'MEX', MY: 'MYS', MZ: 'MOZ', NA: 'NAM',
  NC: 'NCL', NE: 'NER', NF: 'NFK', NG: 'NGA', NI: 'NIC', NL: 'NLD', NO: 'NOR', NP: 'NPL',
  NR: 'NRU', NU: 'NIU', NZ: 'NZL', OM: 'OMN', PA: 'PAN', PE: 'PER', PF: 'PYF', PG: 'PNG',
  PH: 'PHL', PK: 'PAK', PL: 'POL', PM: 'SPM', PN: 'PCN', PR: 'PRI', PS: 'PSE', PT: 'PRT',
  PW: 'PLW', PY: 'PRY', QA: 'QAT', RE: 'REU', RO: 'ROU', RS: 'SRB', RU: 'RUS', RW: 'RWA',
  SA: 'SAU', SB: 'SLB', SC: 'SYC', SD: 'SDN', SE: 'SWE', SG: 'SGP', SH: 'SHN', SI: 'SVN',
  SJ: 'SJM', SK: 'SVK', SL: 'SLE', SM: 'SMR', SN: 'SEN', SO: 'SOM', SR: 'SUR', SS: 'SSD',
  ST: 'STP', SV: 'SLV', SX: 'SXM', SY: 'SYR', SZ: 'SWZ', TC: 'TCA', TD: 'TCD', TF: 'ATF',
  TG: 'TGO', TH: 'THA', TJ: 'TJK', TK: 'TKL', TL: 'TLS', TM: 'TKM', TN: 'TUN', TO: 'TON',
  TR: 'TUR', TT: 'TTO', TV: 'TUV', TW: 'TWN', TZ: 'TZA', UA: 'UKR', UG: 'UGA', UM: 'UMI',
  US: 'USA', UY: 'URY', UZ: 'UZB', VA: 'VAT', VC: 'VCT', VE: 'VEN', VG: 'VGB', VI: 'VIR',
  VN: 'VNM', VU: 'VUT', WF: 'WLF', WS: 'WSM', XK: 'XKX', YE: 'YEM', YT: 'MYT', ZA: 'ZAF',
  ZM: 'ZMB', ZW: 'ZWE',
}

export const ISO3_TO_ISO2: Record<string, string> = Object.fromEntries(
  Object.entries(ISO2_TO_ISO3).map(([iso2, iso3]) => [iso3, iso2])
)

export const ISO3_TO_NAME: Record<string, string> = {
  ABW: 'Aruba', AFG: 'Afghanistan', AGO: 'Angola', AIA: 'Anguilla', ALA: 'Åland Islands',
  ALB: 'Albania', AND: 'Andorra', ARE: 'United Arab Emirates', ARG: 'Argentina', ARM: 'Armenia',
  ASM: 'American Samoa', ATA: 'Antarctica', ATF: 'French Southern Territories',
  ATG: 'Antigua & Barbuda', AUS: 'Australia', AUT: 'Austria', AZE: 'Azerbaijan', BDI: 'Burundi',
  BEL: 'Belgium', BEN: 'Benin', BES: 'Caribbean Netherlands', BFA: 'Burkina Faso',
  BGD: 'Bangladesh', BGR: 'Bulgaria', BHR: 'Bahrain', BHS: 'Bahamas',
  BIH: 'Bosnia & Herzegovina', BLM: 'St. Barthélemy', BLR: 'Belarus', BLZ: 'Belize',
  BMU: 'Bermuda', BOL: 'Bolivia', BRA: 'Brazil', BRB: 'Barbados', BRN: 'Brunei', BTN: 'Bhutan',
  BVT: 'Bouvet Island', BWA: 'Botswana', CAF: 'Central African Republic', CAN: 'Canada',
  CCK: 'Cocos (Keeling) Islands', CHE: 'Switzerland', CHL: 'Chile', CHN: 'China',
  CIV: 'Ivory Coast', CMR: 'Cameroon', COD: 'DR Congo', COG: 'Congo', COK: 'Cook Islands',
  COL: 'Colombia', COM: 'Comoros', CPV: 'Cape Verde', CRI: 'Costa Rica', CUB: 'Cuba',
  CUW: 'Curaçao', CXR: 'Christmas Island', CYM: 'Cayman Islands', CYP: 'Cyprus', CZE: 'Czechia',
  DEU: 'Germany', DJI: 'Djibouti', DMA: 'Dominica', DNK: 'Denmark', DOM: 'Dominican Republic',
  DZA: 'Algeria', ECU: 'Ecuador', EGY: 'Egypt', ERI: 'Eritrea', ESH: 'Western Sahara',
  ESP: 'Spain', EST: 'Estonia', ETH: 'Ethiopia', FIN: 'Finland', FJI: 'Fiji',
  FLK: 'Falkland Islands', FRA: 'France', FRO: 'Faroe Islands', FSM: 'Micronesia', GAB: 'Gabon',
  GBR: 'United Kingdom', GEO: 'Georgia', GGY: 'Guernsey', GHA: 'Ghana', GIB: 'Gibraltar',
  GIN: 'Guinea', GLP: 'Guadeloupe', GMB: 'Gambia', GNB: 'Guinea-Bissau',
  GNQ: 'Equatorial Guinea', GRC: 'Greece', GRD: 'Grenada', GRL: 'Greenland', GTM: 'Guatemala',
  GUF: 'French Guiana', GUM: 'Guam', GUY: 'Guyana', HKG: 'Hong Kong',
  HMD: 'Heard & McDonald Islands', HND: 'Honduras', HRV: 'Croatia', HTI: 'Haiti',
  HUN: 'Hungary', IDN: 'Indonesia', IMN: 'Isle of Man', IND: 'India',
  IOT: 'British Indian Ocean Territory', IRL: 'Ireland', IRN: 'Iran', IRQ: 'Iraq',
  ISL: 'Iceland', ISR: 'Israel', ITA: 'Italy', JAM: 'Jamaica', JEY: 'Jersey', JOR: 'Jordan',
  JPN: 'Japan', KAZ: 'Kazakhstan', KEN: 'Kenya', KGZ: 'Kyrgyzstan', KHM: 'Cambodia',
  KIR: 'Kiribati', KNA: 'St. Kitts & Nevis', KOR: 'South Korea', KWT: 'Kuwait', LAO: 'Laos',
  LBN: 'Lebanon', LBR: 'Liberia', LBY: 'Libya', LCA: 'St. Lucia', LIE: 'Liechtenstein',
  LKA: 'Sri Lanka', LSO: 'Lesotho', LTU: 'Lithuania', LUX: 'Luxembourg', LVA: 'Latvia',
  MAC: 'Macao', MAF: 'St. Martin', MAR: 'Morocco', MCO: 'Monaco', MDA: 'Moldova',
  MDG: 'Madagascar', MDV: 'Maldives', MEX: 'Mexico', MHL: 'Marshall Islands',
  MKD: 'North Macedonia', MLI: 'Mali', MLT: 'Malta', MMR: 'Myanmar', MNE: 'Montenegro',
  MNG: 'Mongolia', MNP: 'Northern Mariana Islands', MOZ: 'Mozambique', MRT: 'Mauritania',
  MSR: 'Montserrat', MTQ: 'Martinique', MUS: 'Mauritius', MWI: 'Malawi', MYS: 'Malaysia',
  MYT: 'Mayotte', NAM: 'Namibia', NCL: 'New Caledonia', NER: 'Niger', NFK: 'Norfolk Island',
  NGA: 'Nigeria', NIC: 'Nicaragua', NIU: 'Niue', NLD: 'Netherlands', NOR: 'Norway',
  NPL: 'Nepal', NRU: 'Nauru', NZL: 'New Zealand', OMN: 'Oman', PAK: 'Pakistan', PAN: 'Panama',
  PCN: 'Pitcairn Islands', PER: 'Peru', PHL: 'Philippines', PLW: 'Palau',
  PNG: 'Papua New Guinea', POL: 'Poland', PRI: 'Puerto Rico', PRK: 'North Korea',
  PRT: 'Portugal', PRY: 'Paraguay', PSE: 'Palestine', PYF: 'French Polynesia', QAT: 'Qatar',
  REU: 'Réunion', ROU: 'Romania', RUS: 'Russia', RWA: 'Rwanda', SAU: 'Saudi Arabia',
  SDN: 'Sudan', SEN: 'Senegal', SGP: 'Singapore', SGS: 'South Georgia & South Sandwich Islands',
  SHN: 'St. Helena', SJM: 'Svalbard & Jan Mayen', SLB: 'Solomon Islands', SLE: 'Sierra Leone',
  SLV: 'El Salvador', SMR: 'San Marino', SOM: 'Somalia', SPM: 'St. Pierre & Miquelon',
  SRB: 'Serbia', SSD: 'South Sudan', STP: 'São Tomé & Príncipe', SUR: 'Suriname',
  SVK: 'Slovakia', SVN: 'Slovenia', SWE: 'Sweden', SWZ: 'Eswatini', SXM: 'Sint Maarten',
  SYC: 'Seychelles', SYR: 'Syria', TCA: 'Turks & Caicos Islands', TCD: 'Chad', TGO: 'Togo',
  THA: 'Thailand', TJK: 'Tajikistan', TKL: 'Tokelau', TKM: 'Turkmenistan', TLS: 'Timor-Leste',
  TON: 'Tonga', TTO: 'Trinidad & Tobago', TUN: 'Tunisia', TUR: 'Türkiye', TUV: 'Tuvalu',
  TWN: 'Taiwan', TZA: 'Tanzania', UGA: 'Uganda', UKR: 'Ukraine', UMI: 'U.S. Outlying Islands',
  URY: 'Uruguay', USA: 'United States', UZB: 'Uzbekistan', VAT: 'Vatican City',
  VCT: 'St. Vincent & Grenadines', VEN: 'Venezuela', VGB: 'British Virgin Islands',
  VIR: 'U.S. Virgin Islands', VNM: 'Vietnam', VUT: 'Vanuatu', WLF: 'Wallis & Futuna',
  WSM: 'Samoa', XKX: 'Kosovo', YEM: 'Yemen', ZAF: 'South Africa', ZMB: 'Zambia',
  ZWE: 'Zimbabwe',
}

/**
 * Display name for a country code. Accepts alpha-2 (what the SDK stores) or
 * alpha-3 (what the map hands back), and returns the code unchanged when it is
 * not a country we know, so an unexpected value still renders as something.
 */
export function countryName(isoCode: string): string {
  const code = isoCode?.trim().toUpperCase() ?? ''
  if (!code) return ''
  if (ISO3_TO_NAME[code]) return ISO3_TO_NAME[code]
  const iso3 = ISO2_TO_ISO3[code]
  return (iso3 && ISO3_TO_NAME[iso3]) || isoCode
}
