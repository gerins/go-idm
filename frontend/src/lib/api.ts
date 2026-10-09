import * as App from '../../wailsjs/go/main/App'
import type { AddRequest, Config, ExternalAdd, Info, Integration, ProbeResult } from './types'

// The generated bindings use class types with helper methods; the wire format
// is plain JSON, so cast to our structural types at the boundary.
const as = <T>(v: unknown) => v as T

export const api = {
  list: () => App.ListDownloads().then((v) => as<Info[]>(v)),
  add: (req: AddRequest) => App.AddDownload(as(req)).then((v) => as<Info>(v)),
  probe: (url: string, headers: Record<string, string> = {}) =>
    App.ProbeURL(url, headers).then((v) => as<ProbeResult>(v)),
  pause: (id: string) => App.PauseDownload(id),
  resume: (id: string) => App.ResumeDownload(id),
  pauseAll: () => App.PauseAll(),
  resumeAll: () => App.ResumeAll(),
  remove: (id: string, deleteFiles: boolean) => App.RemoveDownload(id, deleteFiles),
  openFile: (id: string) => App.OpenFile(id),
  showInFolder: (id: string) => App.ShowInFolder(id),
  getConfig: () => App.GetConfig().then((v) => as<Config>(v)),
  saveConfig: (c: Config) => App.SaveConfig(as(c)).then((v) => as<Config>(v)),
  frontendReady: () => App.FrontendReady().then((v) => as<ExternalAdd[]>(v ?? [])),
  integration: () => App.GetIntegration().then((v) => as<Integration>(v)),
  installIntegration: () => App.InstallIntegration().then((v) => as<Integration>(v)),
  removeIntegration: () => App.RemoveIntegration().then((v) => as<Integration>(v)),
  revealExtension: () => App.RevealExtensionFolder(),
  chooseFolder: (current: string) => App.ChooseFolder(current),
}

export function errMsg(e: unknown): string {
  if (typeof e === 'string') return e
  if (e instanceof Error) return e.message
  return String(e)
}
