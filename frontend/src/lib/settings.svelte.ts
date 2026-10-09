import { api } from './api'
import type { Config } from './types'

class Settings {
  config = $state.raw<Config | null>(null)

  async load() {
    this.config = await api.getConfig()
  }

  async save(next: Config) {
    this.config = await api.saveConfig(next)
  }
}

export const settings = new Settings()
