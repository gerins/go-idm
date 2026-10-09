import { WindowSetDarkTheme, WindowSetLightTheme, WindowSetSystemDefaultTheme } from '../../wailsjs/runtime/runtime'

export type ThemePref = 'system' | 'light' | 'dark'

const KEY = 'goidm.theme'
const isPref = (v: unknown): v is ThemePref => v === 'system' || v === 'light' || v === 'dark'

function load(): ThemePref {
  try {
    const v = localStorage.getItem(KEY)
    if (isPref(v)) return v
  } catch {
    // storage unavailable; fall back to following the OS
  }
  return 'system'
}

class Theme {
  pref = $state<ThemePref>('system')
  #systemDark = $state(true)

  resolved = $derived<'light' | 'dark'>(this.pref === 'system' ? (this.#systemDark ? 'dark' : 'light') : this.pref)

  init() {
    const mq = matchMedia('(prefers-color-scheme: dark)')
    this.#systemDark = mq.matches
    this.pref = load()
    mq.addEventListener('change', (e) => {
      this.#systemDark = e.matches
      this.#apply()
    })
    this.#apply()
  }

  set(pref: ThemePref) {
    this.pref = pref
    try {
      localStorage.setItem(KEY, pref)
    } catch {
      // not persisted, still applied for this session
    }
    this.#apply()
  }

  #apply() {
    document.documentElement.dataset.theme = this.resolved
    // Native title bar (Windows). These are no-ops on other platforms.
    try {
      if (this.pref === 'light') WindowSetLightTheme()
      else if (this.pref === 'dark') WindowSetDarkTheme()
      else WindowSetSystemDefaultTheme()
    } catch {
      // running outside Wails
    }
  }
}

export const theme = new Theme()
