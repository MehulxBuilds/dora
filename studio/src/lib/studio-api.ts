import { useEffect, useState } from 'react'

export type RecordSummary = { key: string; type: string; preview: string }
export type IndexDefinition = { name: string; prefix: string; field: string }
export type Overview = {
  name: string
  mode: string
  seq: string
  hash: string
  records: RecordSummary[]
  indexes: IndexDefinition[]
}
export type RecordDetail = { value: unknown; type: string; references: string[] }

export async function readStudio<T>(path: string, database = '', signal?: AbortSignal): Promise<T> {
  const suffix = database
    ? `${path.includes('?') ? '&' : '?'}db=${encodeURIComponent(database)}`
    : ''
  const response = await fetch(`/api/${path}${suffix}`, { signal })
  if (!response.headers.get('content-type')?.includes('application/json')) {
    throw new Error('Studio could not reach the database. Check the local server and try Refresh.')
  }
  const text = await response.text()
  if (!text.trim())
    throw new Error(
      'The local API returned an empty response. Start dora studio --no-open on port 7432, then refresh. If it is already running, restart it with the latest CLI build.',
    )
  let data
  try {
    data = JSON.parse(text)
  } catch {
    throw new Error(
      'The local API returned invalid JSON. Restart the Go Studio server and refresh.',
    )
  }
  if (!response.ok) throw new Error(data.error || 'Could not read the database. Try Refresh.')
  return data as T
}

type Resource<T> = { request: string; data?: T; error?: string }

export function useStudioResource<T>(path: string | null, database = '', generation = 0) {
  const request = JSON.stringify([path, database, generation])
  const [resource, setResource] = useState<Resource<T> | null>(null)
  useEffect(() => {
    if (path === null) return
    const controller = new AbortController()
    readStudio<T>(path, database, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setResource({ request, data })
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          setResource({
            request,
            error: error instanceof Error ? error.message : 'Unable to read database.',
          })
        }
      })
    return () => controller.abort()
  }, [path, database, generation, request])
  const current = resource?.request === request ? resource : null
  return { data: current?.data, error: current?.error, loading: path !== null && !current }
}
