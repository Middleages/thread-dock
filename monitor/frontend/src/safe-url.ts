export function isSafeExternalURL(value: string): boolean {
  try {
    const url = new URL(value)
    return (url.protocol === 'https:' || url.protocol === 'http:') && Boolean(url.hostname)
  } catch {
    return false
  }
}

export function openExternalURL(value: string): boolean {
  if (!isSafeExternalURL(value)) return false
  const runtime = window.runtime?.BrowserOpenURL
  if (runtime) runtime(value)
  else window.open(value, '_blank', 'noopener,noreferrer')
  return true
}
