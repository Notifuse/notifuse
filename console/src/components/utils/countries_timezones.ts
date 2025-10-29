import CountriesTimezonesData from './countries_timezones.json'
import map from 'lodash/map'
import { TIMEZONE_OPTIONS, VALID_TIMEZONES } from '../../lib/timezones'

// convert to arrays
type Country = {
  name: string
  abbr: string
  zones: string[]
}

// Use the backend-synchronized timezone list
export const Timezones = VALID_TIMEZONES
export const TimezonesFormOptions = TIMEZONE_OPTIONS

export const CountriesMap: Record<string, Country> = CountriesTimezonesData.countries
export const Countries = map(CountriesTimezonesData.countries, (x) => x)
export const CountriesFormOptions = map(CountriesTimezonesData.countries, (x) => {
  return {
    value: x.abbr,
    label: x.abbr + ' - ' + x.name
  }
})

export default CountriesTimezonesData
