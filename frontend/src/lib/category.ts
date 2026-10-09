import type { IconName } from '../components/icons'

interface CategoryMeta {
  icon: IconName
  color: string
}

export const categoryMeta: Record<string, CategoryMeta> = {
  Video: { icon: 'film', color: '#a78bfa' },
  Music: { icon: 'music', color: '#f472b6' },
  Images: { icon: 'image', color: '#34d399' },
  Documents: { icon: 'doc', color: '#60a5fa' },
  Compressed: { icon: 'archive', color: '#fbbf24' },
  Programs: { icon: 'package', color: '#22d3ee' },
  General: { icon: 'file', color: '#94a3b8' },
}

export function metaFor(category: string): CategoryMeta {
  return categoryMeta[category] ?? categoryMeta.General
}

// Keep in sync with internal/engine/category.go.
const byExt: Record<string, string> = {}
for (const [cat, exts] of Object.entries({
  Video: 'mp4 mkv avi mov wmv flv webm m4v mpg mpeg ts',
  Music: 'mp3 flac wav aac ogg m4a wma opus',
  Images: 'jpg jpeg png gif webp bmp svg heic tiff',
  Documents: 'pdf doc docx xls xlsx ppt pptx txt epub rtf csv odt',
  Compressed: 'zip rar 7z tar gz bz2 xz tgz zst',
  Programs: 'exe msi dmg pkg deb rpm apk appimage iso',
})) {
  for (const e of exts.split(' ')) byExt[e] = cat
}

export function categoryFor(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  if (dot < 0) return 'General'
  return byExt[fileName.slice(dot + 1).toLowerCase()] ?? 'General'
}
