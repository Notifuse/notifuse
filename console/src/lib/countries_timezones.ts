import CountriesTimezonesData from './countries_timezones.json'
import { map } from 'lodash'

// convert to arrays
type Country = {
  name: string
  abbr: string
  zones: string[]
}

export const CountriesMap: Record<string, Country> = CountriesTimezonesData.countries
export const Countries = map(CountriesTimezonesData.countries, (x) => x)
export const CountriesFormOptions = map(CountriesTimezonesData.countries, (x) => {
  return {
    value: x.abbr,
    label: x.abbr + ' - ' + x.name
  }
})

export default CountriesTimezonesData
