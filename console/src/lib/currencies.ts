export type Currency = {
  value: string
  label: string
  code: string
  number: string
  digits: number
  currency: string
  countries: string[]
}

// only currencies from http://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml
export const Currencies: Currency[] = [
  {
    value: 'USD',
    label: 'USD - United States dollar',
    code: 'USD',
    number: '840',
    digits: 2,
    currency: 'United States dollar',
    countries: [
      'american samoa',
      'barbados',
      'bermuda',
      'british indian ocean territory',
      'british virgin islands, caribbean netherlands',
      'ecuador',
      'el salvador',
      'guam',
      'haiti',
      'marshall islands',
      'federated states of micronesia',
      'northern mariana islands',
      'palau',
      'panama',
      'puerto rico',
      'timor-leste',
      'turks and caicos islands',
      'united states',
      'u.s. virgin islands',
      'zimbabwe'
    ]
  },
  {
    value: 'EUR',
    label: 'EUR - Euro',
    code: 'EUR',
    number: '978',
    digits: 2,
    currency: 'Euro',
    countries: [
      'andorra',
      'austria',
      'belgium',
      'cyprus',
      'estonia',
      'finland',
      'france',
      'germany',
      'greece',
      'ireland',
      'italy',
      'kosovo',
      'luxembourg',
      'malta',
      'monaco',
      'montenegro',
      'netherlands',
      'portugal',
      'san marino',
      'slovakia',
      'slovenia',
      'spain',
      'vatican city'
    ]
  },
  {
    value: 'GBP',
    label: 'GBP - Pound sterling',
    code: 'GBP',
    number: '826',
    digits: 2,
    currency: 'Pound sterling',
    countries: [
      'united kingdom',
      'british crown dependencies (the  isle of man and the channel islands)',
      'south georgia and the south sandwich islands',
      'british antarctic territory',
      'british indian ocean territory'
    ]
  },
  {
    value: 'CAD',
    label: 'CAD - Canadian dollar',
    code: 'CAD',
    number: '124',
    digits: 2,
    currency: 'Canadian dollar',
    countries: ['canada', 'saint pierre and miquelon']
  },
  {
    value: 'AUD',
    label: 'AUD - Australian dollar',
    code: 'AUD',
    number: '036',
    digits: 2,
    currency: 'Australian dollar',
    countries: [
      'australia',
      'australian antarctic territory',
      'christmas island',
      'cocos (keeling) islands',
      'heard and mcdonald islands',
      'kiribati',
      'nauru',
      'norfolk island',
      'tuvalu'
    ]
  },
  {
    value: 'SGD',
    label: 'SGD - Singapore dollar',
    code: 'SGD',
    number: '702',
    digits: 2,
    currency: 'Singapore dollar',
    countries: ['singapore', 'brunei']
  },
  {
    value: 'JPY',
    label: 'JPY - Japanese yen',
    code: 'JPY',
    number: '392',
    digits: 0,
    currency: 'Japanese yen',
    countries: ['japan']
  },
  {
    value: 'BGN',
    label: 'BGN - Bulgarian lev',
    code: 'BGN',
    number: '975',
    digits: 2,
    currency: 'Bulgarian lev',
    countries: ['bulgaria']
  },
  {
    value: 'CZK',
    label: 'CZK - Czech koruna',
    code: 'CZK',
    number: '203',
    digits: 2,
    currency: 'Czech koruna',
    countries: ['czech republic']
  },
  {
    value: 'DKK',
    label: 'DKK - Danish krone',
    code: 'DKK',
    number: '208',
    digits: 2,
    currency: 'Danish krone',
    countries: ['denmark', 'faroe islands', 'greenland']
  },
  {
    value: 'HUF',
    label: 'HUF - Hungarian forint',
    code: 'HUF',
    number: '348',
    digits: 2,
    currency: 'Hungarian forint',
    countries: ['hungary']
  },
  {
    value: 'PLN',
    label: 'PLN - Polish złoty',
    code: 'PLN',
    number: '985',
    digits: 2,
    currency: 'Polish złoty',
    countries: ['poland']
  },
  {
    value: 'RON',
    label: 'RON - Romanian new leu',
    code: 'RON',
    number: '946',
    digits: 2,
    currency: 'Romanian new leu',
    countries: ['romania']
  },
  {
    value: 'SEK',
    label: 'SEK - Swedish krona/kronor',
    code: 'SEK',
    number: '752',
    digits: 2,
    currency: 'Swedish krona/kronor',
    countries: ['sweden']
  },
  {
    value: 'CHF',
    label: 'CHF - Swiss franc',
    code: 'CHF',
    number: '756',
    digits: 2,
    currency: 'Swiss franc',
    countries: ['switzerland', 'liechtenstein']
  },
  {
    value: 'ISK',
    label: 'ISK - Icelandic króna',
    code: 'ISK',
    number: '352',
    digits: 0,
    currency: 'Icelandic króna',
    countries: ['iceland']
  },
  {
    value: 'NOK',
    label: 'NOK - Norwegian krone',
    code: 'NOK',
    number: '578',
    digits: 2,
    currency: 'Norwegian krone',
    countries: [
      'norway',
      'svalbard',
      'jan mayen',
      'bouvet island',
      'queen maud land',
      'peter i island'
    ]
  },
  {
    value: 'HRK',
    label: 'HRK - Croatian kuna',
    code: 'HRK',
    number: '191',
    digits: 2,
    currency: 'Croatian kuna',
    countries: ['croatia']
  },
  {
    value: 'RUB',
    label: 'RUB - Russian rouble',
    code: 'RUB',
    number: '643',
    digits: 2,
    currency: 'Russian rouble',
    countries: ['russia', 'abkhazia', 'south ossetia']
  },
  {
    value: 'TRY',
    label: 'TRY - Turkish lira',
    code: 'TRY',
    number: '949',
    digits: 2,
    currency: 'Turkish lira',
    countries: ['turkey', 'northern cyprus']
  },
  {
    value: 'BRL',
    label: 'BRL - Brazilian real',
    code: 'BRL',
    number: '986',
    digits: 2,
    currency: 'Brazilian real',
    countries: ['brazil']
  },
  {
    value: 'CNY',
    label: 'CNY - Chinese yuan',
    code: 'CNY',
    number: '156',
    digits: 2,
    currency: 'Chinese yuan',
    countries: ['china']
  },
  {
    value: 'HKD',
    label: 'HKD - Hong Kong dollar',
    code: 'HKD',
    number: '344',
    digits: 2,
    currency: 'Hong Kong dollar',
    countries: ['hong kong', 'macao']
  },
  {
    value: 'IDR',
    label: 'IDR - Indonesian rupiah',
    code: 'IDR',
    number: '360',
    digits: 0,
    currency: 'Indonesian rupiah',
    countries: ['indonesia']
  },
  {
    value: 'ILS',
    label: 'ILS - Israeli new shekel',
    code: 'ILS',
    number: '376',
    digits: 2,
    currency: 'Israeli new shekel',
    countries: ['israel', 'palestinian territories']
  },
  {
    value: 'INR',
    label: 'INR - Indian rupee',
    code: 'INR',
    number: '356',
    digits: 2,
    currency: 'Indian rupee',
    countries: ['india']
  },
  {
    value: 'KRW',
    label: 'KRW - South Korean won',
    code: 'KRW',
    number: '410',
    digits: 0,
    currency: 'South Korean won',
    countries: ['south korea']
  },
  {
    value: 'MXN',
    label: 'MXN - Mexican peso',
    code: 'MXN',
    number: '484',
    digits: 2,
    currency: 'Mexican peso',
    countries: ['mexico']
  },
  {
    value: 'MYR',
    label: 'MYR - Malaysian ringgit',
    code: 'MYR',
    number: '458',
    digits: 2,
    currency: 'Malaysian ringgit',
    countries: ['malaysia']
  },
  {
    value: 'NZD',
    label: 'NZD - New Zealand dollar',
    code: 'NZD',
    number: '554',
    digits: 2,
    currency: 'New Zealand dollar',
    countries: ['cook islands', 'new zealand', 'niue', 'pitcairn', 'tokelau', 'ross dependency']
  },
  {
    value: 'PHP',
    label: 'PHP - Philippine peso',
    code: 'PHP',
    number: '608',
    digits: 2,
    currency: 'Philippine peso',
    countries: ['philippines']
  },
  {
    value: 'THB',
    label: 'THB - Thai baht',
    code: 'THB',
    number: '764',
    digits: 2,
    currency: 'Thai baht',
    countries: ['thailand']
  },
  {
    value: 'ZAR',
    label: 'ZAR - South African rand',
    code: 'ZAR',
    number: '710',
    digits: 2,
    currency: 'South African rand',
    countries: ['south africa']
  }

  // {
  //   code: 'AED',
  //   number: '784',
  //   digits: 2,
  //   currency: 'United Arab Emirates dirham',
  //   countries: ['united arab emirates']
  // }, {
  //   code: 'AFN',
  //   number: '971',
  //   digits: 2,
  //   currency: 'Afghan afghani',
  //   countries: ['afghanistan']
  // }, {
  //   code: 'ALL',
  //   number: '008',
  //   digits: 2,
  //   currency: 'Albanian lek',
  //   countries: ['albania']
  // }, {
  //   code: 'AMD',
  //   number: '051',
  //   digits: 2,
  //   currency: 'Armenian dram',
  //   countries: ['armenia']
  // }, {
  //   code: 'ANG',
  //   number: '532',
  //   digits: 2,
  //   currency: 'Netherlands Antillean guilder',
  //   countries: ['curaçao', 'sint maarten']
  // }, {
  //   code: 'AOA',
  //   number: '973',
  //   digits: 2,
  //   currency: 'Angolan kwanza',
  //   countries: ['angola']
  // }, {
  //   code: 'ARS',
  //   number: '032',
  //   digits: 2,
  //   currency: 'Argentine peso',
  //   countries: ['argentina']
  // }, {
  //   code: 'AWG',
  //   number: '533',
  //   digits: 2,
  //   currency: 'Aruban florin',
  //   countries: ['aruba']
  // }, {
  //   code: 'AZN',
  //   number: '944',
  //   digits: 2,
  //   currency: 'Azerbaijani manat',
  //   countries: ['azerbaijan']
  // }, {
  //   code: 'BAM',
  //   number: '977',
  //   digits: 2,
  //   currency: 'Bosnia and Herzegovina convertible mark',
  //   countries: ['bosnia and herzegovina']
  // }, {
  //   code: 'BBD',
  //   number: '052',
  //   digits: 2,
  //   currency: 'Barbados dollar',
  //   countries: ['barbados']
  // }, {
  //   code: 'BDT',
  //   number: '050',
  //   digits: 2,
  //   currency: 'Bangladeshi taka',
  //   countries: ['bangladesh']
  // }, {
  //   code: 'BHD',
  //   number: '048',
  //   digits: 3,
  //   currency: 'Bahraini dinar',
  //   countries: ['bahrain']
  // }, {
  //   code: 'BIF',
  //   number: '108',
  //   digits: 0,
  //   currency: 'Burundian franc',
  //   countries: ['burundi']
  // }, {
  //   code: 'BMD',
  //   number: '060',
  //   digits: 2,
  //   currency: 'Bermudian dollar',
  //   countries: ['bermuda']
  // }, {
  //   code: 'BND',
  //   number: '096',
  //   digits: 2,
  //   currency: 'Brunei dollar',
  //   countries: ['brunei', 'singapore']
  // }, {
  //   code: 'BOB',
  //   number: '068',
  //   digits: 2,
  //   currency: 'Boliviano',
  //   countries: ['bolivia']
  // }, {
  //   code: 'BOV',
  //   number: '984',
  //   digits: 2,
  //   currency: 'Bolivian Mvdol (funds code)',
  //   countries: ['bolivia']
  // }, {
  //   code: 'BSD',
  //   number: '044',
  //   digits: 2,
  //   currency: 'Bahamian dollar',
  //   countries: ['bahamas']
  // }, {
  //   code: 'BTN',
  //   number: '064',
  //   digits: 2,
  //   currency: 'Bhutanese ngultrum',
  //   countries: ['bhutan']
  // }, {
  //   code: 'BWP',
  //   number: '072',
  //   digits: 2,
  //   currency: 'Botswana pula',
  //   countries: ['botswana']
  // }, {
  //   code: 'BYR',
  //   number: '974',
  //   digits: 0,
  //   currency: 'Belarusian ruble',
  //   countries: ['belarus']
  // }, {
  //   code: 'BZD',
  //   number: '084',
  //   digits: 2,
  //   currency: 'Belize dollar',
  //   countries: ['belize']
  // }, {
  //   code: 'CDF',
  //   number: '976',
  //   digits: 2,
  //   currency: 'Congolese franc',
  //   countries: ['democratic republic of congo']
  // }, {
  //   code: 'CHE',
  //   number: '947',
  //   digits: 2,
  //   currency: 'WIR Euro (complementary currency)',
  //   countries: ['switzerland']
  // }, {
  //   code: 'CHW',
  //   number: '948',
  //   digits: 2,
  //   currency: 'WIR Franc (complementary currency)',
  //   countries: ['switzerland']
  // }, {
  //   code: 'CLF',
  //   number: '990',
  //   digits: 0,
  //   currency: 'Unidad de Fomento (funds code)',
  //   countries: ['chile']
  // }, {
  //   code: 'CLP',
  //   number: '152',
  //   digits: 0,
  //   currency: 'Chilean peso',
  //   countries: ['chile']
  // }, {
  //   code: 'COP',
  //   number: '170',
  //   digits: 2,
  //   currency: 'Colombian peso',
  //   countries: ['colombia']
  // }, {
  //   code: 'COU',
  //   number: '970',
  //   digits: 2,
  //   currency: 'Unidad de Valor Real',
  //   countries: ['colombia']
  // }, {
  //   code: 'CRC',
  //   number: '188',
  //   digits: 2,
  //   currency: 'Costa Rican colon',
  //   countries: ['costa rica']
  // }, {
  //   code: 'CUC',
  //   number: '931',
  //   digits: 2,
  //   currency: 'Cuban convertible peso',
  //   countries: ['cuba']
  // }, {
  //   code: 'CUP',
  //   number: '192',
  //   digits: 2,
  //   currency: 'Cuban peso',
  //   countries: ['cuba']
  // }, {
  //   code: 'CVE',
  //   number: '132',
  //   digits: 0,
  //   currency: 'Cape Verde escudo',
  //   countries: ['cape verde']
  // }, {
  //   code: 'DJF',
  //   number: '262',
  //   digits: 0,
  //   currency: 'Djiboutian franc',
  //   countries: ['djibouti']
  // }, {
  //   code: 'DOP',
  //   number: '214',
  //   digits: 2,
  //   currency: 'Dominican peso',
  //   countries: ['dominican republic']
  // }, {
  //   code: 'DZD',
  //   number: '012',
  //   digits: 2,
  //   currency: 'Algerian dinar',
  //   countries: ['algeria']
  // }, {
  //   code: 'EGP',
  //   number: '818',
  //   digits: 2,
  //   currency: 'Egyptian pound',
  //   countries: ['egypt', 'palestinian territories']
  // }, {
  //   code: 'ERN',
  //   number: '232',
  //   digits: 2,
  //   currency: 'Eritrean nakfa',
  //   countries: ['eritrea']
  // }, {
  //   code: 'ETB',
  //   number: '230',
  //   digits: 2,
  //   currency: 'Ethiopian birr',
  //   countries: ['ethiopia']
  // }, {
  //   code: 'FJD',
  //   number: '242',
  //   digits: 2,
  //   currency: 'Fiji dollar',
  //   countries: ['fiji']
  // }, {
  //   code: 'FKP',
  //   number: '238',
  //   digits: 2,
  //   currency: 'Falkland Islands pound',
  //   countries: ['falkland islands']
  // }, {
  //   code: 'GEL',
  //   number: '981',
  //   digits: 2,
  //   currency: 'Georgian lari',
  //   countries: ['georgia']
  // }, {
  //   code: 'GHS',
  //   number: '936',
  //   digits: 2,
  //   currency: 'Ghanaian cedi',
  //   countries: ['ghana']
  // }, {
  //   code: 'GIP',
  //   number: '292',
  //   digits: 2,
  //   currency: 'Gibraltar pound',
  //   countries: ['gibraltar']
  // }, {
  //   code: 'GMD',
  //   number: '270',
  //   digits: 2,
  //   currency: 'Gambian dalasi',
  //   countries: ['gambia']
  // }, {
  //   code: 'GNF',
  //   number: '324',
  //   digits: 0,
  //   currency: 'Guinean franc',
  //   countries: ['guinea']
  // }, {
  //   code: 'GTQ',
  //   number: '320',
  //   digits: 2,
  //   currency: 'Guatemalan quetzal',
  //   countries: ['guatemala']
  // }, {
  //   code: 'GYD',
  //   number: '328',
  //   digits: 2,
  //   currency: 'Guyanese dollar',
  //   countries: ['guyana']
  // }, {
  //   code: 'HNL',
  //   number: '340',
  //   digits: 2,
  //   currency: 'Honduran lempira',
  //   countries: ['honduras']
  // }, {
  //   code: 'HTG',
  //   number: '332',
  //   digits: 2,
  //   currency: 'Haitian gourde',
  //   countries: ['haiti']
  // }, {
  //   code: 'IQD',
  //   number: '368',
  //   digits: 3,
  //   currency: 'Iraqi dinar',
  //   countries: ['iraq']
  // }, {
  //   code: 'IRR',
  //   number: '364',
  //   digits: 0,
  //   currency: 'Iranian rial',
  //   countries: ['iran']
  // }, {
  //   code: 'JMD',
  //   number: '388',
  //   digits: 2,
  //   currency: 'Jamaican dollar',
  //   countries: ['jamaica']
  // }, {
  //   code: 'JOD',
  //   number: '400',
  //   digits: 3,
  //   currency: 'Jordanian dinar',
  //   countries: ['jordan']
  // }, {
  //   code: 'KES',
  //   number: '404',
  //   digits: 2,
  //   currency: 'Kenyan shilling',
  //   countries: ['kenya']
  // }, {
  //   code: 'KGS',
  //   number: '417',
  //   digits: 2,
  //   currency: 'Kyrgyzstani som',
  //   countries: ['kyrgyzstan']
  // }, {
  //   code: 'KHR',
  //   number: '116',
  //   digits: 2,
  //   currency: 'Cambodian riel',
  //   countries: ['cambodia']
  // }, {
  //   code: 'KMF',
  //   number: '174',
  //   digits: 0,
  //   currency: 'Comoro franc',
  //   countries: ['comoros']
  // }, {
  //   code: 'KPW',
  //   number: '408',
  //   digits: 0,
  //   currency: 'North Korean won',
  //   countries: ['north korea']
  // }, {
  //   code: 'KWD',
  //   number: '414',
  //   digits: 3,
  //   currency: 'Kuwaiti dinar',
  //   countries: ['kuwait']
  // }, {
  //   code: 'KYD',
  //   number: '136',
  //   digits: 2,
  //   currency: 'Cayman Islands dollar',
  //   countries: ['cayman islands']
  // }, {
  //   code: 'KZT',
  //   number: '398',
  //   digits: 2,
  //   currency: 'Kazakhstani tenge',
  //   countries: ['kazakhstan']
  // }, {
  //   code: 'LAK',
  //   number: '418',
  //   digits: 0,
  //   currency: 'Lao kip',
  //   countries: ['laos']
  // }, {
  //   code: 'LBP',
  //   number: '422',
  //   digits: 0,
  //   currency: 'Lebanese pound',
  //   countries: ['lebanon']
  // }, {
  //   code: 'LKR',
  //   number: '144',
  //   digits: 2,
  //   currency: 'Sri Lankan rupee',
  //   countries: ['sri lanka']
  // }, {
  //   code: 'LRD',
  //   number: '430',
  //   digits: 2,
  //   currency: 'Liberian dollar',
  //   countries: ['liberia']
  // }, {
  //   code: 'LSL',
  //   number: '426',
  //   digits: 2,
  //   currency: 'Lesotho loti',
  //   countries: ['lesotho']
  // }, {
  //   code: 'LTL',
  //   number: '440',
  //   digits: 2,
  //   currency: 'Lithuanian litas',
  //   countries: ['lithuania']
  // }, {
  //   code: 'LVL',
  //   number: '428',
  //   digits: 2,
  //   currency: 'Latvian lats',
  //   countries: ['latvia']
  // }, {
  //   code: 'LYD',
  //   number: '434',
  //   digits: 3,
  //   currency: 'Libyan dinar',
  //   countries: ['libya']
  // }, {
  //   code: 'MAD',
  //   number: '504',
  //   digits: 2,
  //   currency: 'Moroccan dirham',
  //   countries: ['morocco']
  // }, {
  //   code: 'MDL',
  //   number: '498',
  //   digits: 2,
  //   currency: 'Moldovan leu',
  //   countries: ['moldova (except  transnistria)']
  // }, {
  //   code: 'MGA',
  //   number: '969',
  //   digits: 0,
  //   currency: '*[8] Malagasy ariary',
  //   countries: ['madagascar']
  // }, {
  //   code: 'MKD',
  //   number: '807',
  //   digits: 0,
  //   currency: 'Macedonian denar',
  //   countries: ['macedonia']
  // }, {
  //   code: 'MMK',
  //   number: '104',
  //   digits: 0,
  //   currency: 'Myanma kyat',
  //   countries: ['myanmar']
  // }, {
  //   code: 'MNT',
  //   number: '496',
  //   digits: 2,
  //   currency: 'Mongolian tugrik',
  //   countries: ['mongolia']
  // }, {
  //   code: 'MOP',
  //   number: '446',
  //   digits: 2,
  //   currency: 'Macanese pataca',
  //   countries: ['macao']
  // }, {
  //   code: 'MRO',
  //   number: '478',
  //   digits: 0,
  //   currency: '*[8] Mauritanian ouguiya',
  //   countries: ['mauritania']
  // }, {
  //   code: 'MUR',
  //   number: '480',
  //   digits: 2,
  //   currency: 'Mauritian rupee',
  //   countries: ['mauritius']
  // }, {
  //   code: 'MVR',
  //   number: '462',
  //   digits: 2,
  //   currency: 'Maldivian rufiyaa',
  //   countries: ['maldives']
  // }, {
  //   code: 'MWK',
  //   number: '454',
  //   digits: 2,
  //   currency: 'Malawian kwacha',
  //   countries: ['malawi']
  // }, {
  //   code: 'MXV',
  //   number: '979',
  //   digits: 2,
  //   currency: 'Mexican Unidad de Inversion (UDI) (funds code)',
  //   countries: ['mexico']
  // }, {
  //   code: 'MZN',
  //   number: '943',
  //   digits: 2,
  //   currency: 'Mozambican metical',
  //   countries: ['mozambique']
  // }, {
  //   code: 'NAD',
  //   number: '516',
  //   digits: 2,
  //   currency: 'Namibian dollar',
  //   countries: ['namibia']
  // }, {
  //   code: 'NGN',
  //   number: '566',
  //   digits: 2,
  //   currency: 'Nigerian naira',
  //   countries: ['nigeria']
  // }, {
  //   code: 'NIO',
  //   number: '558',
  //   digits: 2,
  //   currency: 'Nicaraguan córdoba',
  //   countries: ['nicaragua']
  // }, {
  //   code: 'NPR',
  //   number: '524',
  //   digits: 2,
  //   currency: 'Nepalese rupee',
  //   countries: ['nepal']
  // }, {
  //   code: 'OMR',
  //   number: '512',
  //   digits: 3,
  //   currency: 'Omani rial',
  //   countries: ['oman']
  // }, {
  //   code: 'PAB',
  //   number: '590',
  //   digits: 2,
  //   currency: 'Panamanian balboa',
  //   countries: ['panama']
  // }, {
  //   code: 'PEN',
  //   number: '604',
  //   digits: 2,
  //   currency: 'Peruvian nuevo sol',
  //   countries: ['peru']
  // }, {
  //   code: 'PGK',
  //   number: '598',
  //   digits: 2,
  //   currency: 'Papua New Guinean kina',
  //   countries: ['papua new guinea']
  // }, {
  //   code: 'PKR',
  //   number: '586',
  //   digits: 2,
  //   currency: 'Pakistani rupee',
  //   countries: ['pakistan']
  // }, {
  //   code: 'PYG',
  //   number: '600',
  //   digits: 0,
  //   currency: 'Paraguayan guaraní',
  //   countries: ['paraguay']
  // }, {
  //   code: 'QAR',
  //   number: '634',
  //   digits: 2,
  //   currency: 'Qatari riyal',
  //   countries: ['qatar']
  // }, {
  //   code: 'RSD',
  //   number: '941',
  //   digits: 2,
  //   currency: 'Serbian dinar',
  //   countries: ['serbia']
  // }, {
  //   code: 'RWF',
  //   number: '646',
  //   digits: 0,
  //   currency: 'Rwandan franc',
  //   countries: ['rwanda']
  // }, {
  //   code: 'SAR',
  //   number: '682',
  //   digits: 2,
  //   currency: 'Saudi riyal',
  //   countries: ['saudi arabia']
  // }, {
  //   code: 'SBD',
  //   number: '090',
  //   digits: 2,
  //   currency: 'Solomon Islands dollar',
  //   countries: ['solomon islands']
  // }, {
  //   code: 'SCR',
  //   number: '690',
  //   digits: 2,
  //   currency: 'Seychelles rupee',
  //   countries: ['seychelles']
  // }, {
  //   code: 'SDG',
  //   number: '938',
  //   digits: 2,
  //   currency: 'Sudanese pound',
  //   countries: ['sudan']
  // }, {
  //   code: 'SHP',
  //   number: '654',
  //   digits: 2,
  //   currency: 'Saint Helena pound',
  //   countries: ['saint helena']
  // }, {
  //   code: 'SLL',
  //   number: '694',
  //   digits: 0,
  //   currency: 'Sierra Leonean leone',
  //   countries: ['sierra leone']
  // }, {
  //   code: 'SOS',
  //   number: '706',
  //   digits: 2,
  //   currency: 'Somali shilling',
  //   countries: ['somalia']
  // }, {
  //   code: 'SRD',
  //   number: '968',
  //   digits: 2,
  //   currency: 'Surinamese dollar',
  //   countries: ['suriname']
  // }, {
  //   code: 'SSP',
  //   number: '728',
  //   digits: 2,
  //   currency: 'South Sudanese pound',
  //   countries: ['south sudan']
  // }, {
  //   code: 'STD',
  //   number: '678',
  //   digits: 0,
  //   currency: 'São Tomé and Príncipe dobra',
  //   countries: ['são tomé and príncipe']
  // }, {
  //   code: 'SYP',
  //   number: '760',
  //   digits: 2,
  //   currency: 'Syrian pound',
  //   countries: ['syria']
  // }, {
  //   code: 'SZL',
  //   number: '748',
  //   digits: 2,
  //   currency: 'Swazi lilangeni',
  //   countries: ['swaziland']
  // }, {
  //   code: 'TJS',
  //   number: '972',
  //   digits: 2,
  //   currency: 'Tajikistani somoni',
  //   countries: ['tajikistan']
  // }, {
  //   code: 'TMT',
  //   number: '934',
  //   digits: 2,
  //   currency: 'Turkmenistani manat',
  //   countries: ['turkmenistan']
  // }, {
  //   code: 'TND',
  //   number: '788',
  //   digits: 3,
  //   currency: 'Tunisian dinar',
  //   countries: ['tunisia']
  // }, {
  //   code: 'TOP',
  //   number: '776',
  //   digits: 2,
  //   currency: 'Tongan paʻanga',
  //   countries: ['tonga']
  // }, {
  //   code: 'TTD',
  //   number: '780',
  //   digits: 2,
  //   currency: 'Trinidad and Tobago dollar',
  //   countries: ['trinidad and tobago']
  // }, {
  //   code: 'TWD',
  //   number: '901',
  //   digits: 2,
  //   currency: 'New Taiwan dollar',
  //   countries: ['republic of china (taiwan)']
  // }, {
  //   code: 'TZS',
  //   number: '834',
  //   digits: 2,
  //   currency: 'Tanzanian shilling',
  //   countries: ['tanzania']
  // }, {
  //   code: 'UAH',
  //   number: '980',
  //   digits: 2,
  //   currency: 'Ukrainian hryvnia',
  //   countries: ['ukraine']
  // }, {
  //   code: 'UGX',
  //   number: '800',
  //   digits: 2,
  //   currency: 'Ugandan shilling',
  //   countries: ['uganda']
  // }, {
  //   code: 'USN',
  //   number: '997',
  //   digits: 2,
  //   currency: 'United States dollar (next day) (funds code)',
  //   countries: ['united states']
  // }, {
  //   code: 'USS',
  //   number: '998',
  //   digits: 2,
  //   currency: 'United States dollar',
  //   countries: ['united states']
  // }, {
  //   code: 'UYI',
  //   number: '940',
  //   digits: 0,
  //   currency: 'Uruguay Peso en Unidades Indexadas',
  //   countries: ['uruguay']
  // }, {
  //   code: 'UYU',
  //   number: '858',
  //   digits: 2,
  //   currency: 'Uruguayan peso',
  //   countries: ['uruguay']
  // }, {
  //   code: 'UZS',
  //   number: '860',
  //   digits: 2,
  //   currency: 'Uzbekistan som',
  //   countries: ['uzbekistan']
  // }, {
  //   code: 'VEF',
  //   number: '937',
  //   digits: 2,
  //   currency: 'Venezuelan bolívar',
  //   countries: ['venezuela']
  // }, {
  //   code: 'VND',
  //   number: '704',
  //   digits: 0,
  //   currency: 'Vietnamese dong',
  //   countries: ['vietnam']
  // }, {
  //   code: 'VUV',
  //   number: '548',
  //   digits: 0,
  //   currency: 'Vanuatu vatu',
  //   countries: ['vanuatu']
  // }, {
  //   code: 'WST',
  //   number: '882',
  //   digits: 2,
  //   currency: 'Samoan tala',
  //   countries: ['samoa']
  // }, {
  //   code: 'XAF',
  //   number: '950',
  //   digits: 0,
  //   currency: 'CFA franc BEAC',
  //   countries: ['cameroon', 'central african republic', 'republic of the congo', 'chad', 'equatorial guinea', 'gabon']
  // }, {
  //   code: 'XAG',
  //   number: '961',
  //   currency: 'Silver (one troy ounce)',
  // }, {
  //   code: 'XAU',
  //   number: '959',
  //   currency: 'Gold (one troy ounce)',
  // }, {
  //   code: 'XBA',
  //   number: '955',
  //   currency: 'European Composite Unit (EURCO) (bond market unit) ',
  // }, {
  //   code: 'XBB',
  //   number: '956',
  //   currency: 'European Monetary Unit (E.M.U.-6) (bond market unit) ',
  // }, {
  //   code: 'XBC',
  //   number: '957',
  //   currency: 'European Unit of Account 9 (E.U.A.-9) (bond market unit) ',
  // }, {
  //   code: 'XBD',
  //   number: '958',
  //   currency: 'European Unit of Account 17 (E.U.A.-17) (bond market unit) ',
  // }, {
  //   code: 'XBT',
  //   currency: 'Bitcoin',
  // }, {
  //   code: 'XCD',
  //   number: '951',
  //   digits: 2,
  //   currency: 'East Caribbean dollar',
  //   countries: ['anguilla', 'antigua and barbuda', 'dominica', 'grenada', 'montserrat', 'saint kitts and nevis', 'saint lucia', 'saint vincent and the grenadines']
  // }, {
  //   code: 'XDR',
  //   number: '960',
  //   currency: 'Special drawing rights',
  //   countries: ['international monetary fund']
  // }, {
  //   code: 'XFU',
  //   currency: 'UIC franc (special settlement currency)',
  //   countries: ['international union of railways']
  // }, {
  //   code: 'XOF',
  //   number: '952',
  //   digits: 0,
  //   currency: 'CFA franc BCEAO',
  //   countries: ['benin', 'burkina faso', 'côte d\'ivoire', 'guinea-bissau', 'mali', 'niger', 'senegal', 'togo']
  // }, {
  //   code: 'XPD',
  //   number: '964',
  //   currency: 'Palladium (one troy ounce)',
  // }, {
  //   code: 'XPF',
  //   number: '953',
  //   digits: 0,
  //   currency: 'CFP franc (Franc du Pacifique)',
  //   countries: ['french polynesia', 'new caledonia', 'wallis and futuna']
  // }, {
  //   code: 'XPT',
  //   number: '962',
  //   currency: 'Platinum (one troy ounce)',
  // }, {
  //   code: 'XTS',
  //   number: '963',
  //   currency: 'Code reserved for testing purposes',
  // }, {
  //   code: 'XXX',
  //   number: '999',
  //   currency: 'No currency',
  // }, {
  //   code: 'YER',
  //   number: '886',
  //   digits: 2,
  //   currency: 'Yemeni rial',
  //   countries: ['yemen']
  // }, {
  //   code: 'ZMW',
  //   number: '967',
  //   digits: 2,
  //   currency: 'Zambian kwacha',
  //   countries: ['zambia']
  // }
]
