import { useSyncExternalStore } from 'react'

const navigationEvent = 'dora:navigate'

function subscribe(callback: () => void) {
  window.addEventListener('popstate', callback)
  window.addEventListener(navigationEvent, callback)
  return () => {
    window.removeEventListener('popstate', callback)
    window.removeEventListener(navigationEvent, callback)
  }
}

export function useNavigation() {
  const search = useSyncExternalStore(
    subscribe,
    () => window.location.search,
    () => '',
  )
  const params = new URLSearchParams(search)
  function navigate(changes: Record<string, string | null>, push = false) {
    const next = new URLSearchParams(window.location.search)
    for (const [key, value] of Object.entries(changes)) {
      if (value === null || value === '') next.delete(key)
      else next.set(key, value)
    }
    const query = next.toString()
    const address = window.location.pathname + (query ? `?${query}` : '')
    window.history[push ? 'pushState' : 'replaceState'](null, '', address)
    window.dispatchEvent(new Event(navigationEvent))
  }
  function recordHref(key: string) {
    const next = new URLSearchParams(search)
    next.set('key', key)
    return `?${next}`
  }
  return { params, navigate, recordHref }
}
