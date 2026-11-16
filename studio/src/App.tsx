import { useEffect, useRef, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import './App.css'

type RecordSummary = { key: string; type: string; preview: string }
type Index = { name: string; prefix: string; field: string }
type Overview = {
  name: string
  mode: string
  seq: string
  hash: string
  records: RecordSummary[]
  indexes: Index[]
}
type Detail = { 
  value: unknown; type: string; references: string[] 
}

async function api<T>(path: string, signal?: AbortSignal, database = ''): Promise<T> {
  const suffix = database
    ? `${path.includes('?') ? '&' : '?'}db=${encodeURIComponent(database)}`
    : ''
  const response = await fetch(`/api/${path}${suffix}`, { signal })
  const data = await response.json()
  if (!response.ok) throw new Error(data.error || 'Could not read database')
  return data as T
}

function Icon({
  kind,
}: {
  kind: 'database' | 'grid' | 'index' | 'search' | 'refresh' | 'arrow' | 'lock'
}) {
  const paths = {
    database: (
      <>
        <ellipse cx="12" cy="5" rx="8" ry="3" />
        <path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0" />
      </>
    ),
    grid: (
      <>
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
        <rect x="14" y="14" width="7" height="7" rx="1" />
      </>
    ),
    index: (
      <>
        <path d="M8 5h13M8 12h13M8 19h13M3 5h.01M3 12h.01M3 19h.01" />
      </>
    ),
    search: (
      <>
        <circle cx="10" cy="10" r="6" />
        <path d="m15 15 6 6" />
      </>
    ),
    refresh: (
      <>
        <path d="M20 8a8 8 0 1 0 0 8M20 3v5h-5" />
      </>
    ),
    arrow: <path d="M5 12h14m-5-5 5 5-5 5" />,
    lock: (
      <>
        <rect x="5" y="10" width="14" height="11" rx="2" />
        <path d="M8 10V6a4 4 0 0 1 8 0v4" />
      </>
    ),
  }
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[kind]}
    </svg>
  )
}

function JsonValue({
  value,
  onReference,
  depth = 0,
}: {
  value: unknown
  onReference: (key: string) => void
  depth?: number
}): ReactNode {
  if (depth > 30) return <span>{JSON.stringify(value)}</span>
  if (value === null) return <span className="json-null">null</span>
  if (typeof value === 'string') return <span className="json-string">{JSON.stringify(value)}</span>
  if (typeof value !== 'object') return <span className="json-number">{String(value)}</span>
  const object = value as Record<string, unknown>
  if (
    !Array.isArray(value) &&
    Object.keys(object).length === 1 &&
    typeof object['\u0000_ref'] === 'string'
  ) {
    const target = object['\u0000_ref']
    return (
      <button className="reference" onClick={() => onReference(target)} title={`Open ${target}`}>
        &#8599; {target}
      </button>
    )
  }
  // Escaped object fields are literal and must not be interpreted as a reference.
  const escaped =
    !Array.isArray(value) &&
    Object.keys(object).length === 1 &&
    typeof object['\u0000_object'] === 'object' &&
    object['\u0000_object'] !== null
  const data = escaped ? (object['\u0000_object'] as Record<string, unknown>) : object
  const array = Array.isArray(value)
  const entries = Object.entries(data)
  if (!entries.length) return <span>{array ? '[]' : '{}'}</span>
  return (
    <span>
      {array ? '[' : '{'}
      <span className="json-indent">
        {entries.map(([key, item], i) => (
          <span className="json-line" key={key}>
            {!array && (
              <>
                <span className="json-key">{JSON.stringify(key)}</span>:{' '}
              </>
            )}
            <JsonValue value={item} onReference={onReference} depth={depth + 1} />
            {i < entries.length - 1 ? ',' : ''}
          </span>
        ))}
      </span>
      {array ? ']' : '}'}
    </span>
  )
}

export default function App() {
  const [database, setDatabase] = useState('')
  const [databases, setDatabases] = useState<string[]>([])
  const lookupController = useRef<AbortController | null>(null)
  const [overview, setOverview] = useState<Overview | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<'records' | 'indexes'>('records')
  const [search, setSearch] = useState('')
  const [prefix, setPrefix] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [detailError, setDetailError] = useState('')
  const [generation, setGeneration] = useState(0)
  const [page, setPage] = useState(0)
  const [indexName, setIndexName] = useState('')
  const [query, setQuery] = useState('"mehul@example.com"')
  const [matches, setMatches] = useState<string[] | null>(null)
  const [lookupError, setLookupError] = useState('')
  const [querying, setQuerying] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    api<string[]>('databases', controller.signal)
      .then(setDatabases)
      .catch((e) => {
        if (e.name !== 'AbortError') setError(e.message)
      })
    return () => {
      controller.abort()
      lookupController.current?.abort()
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    api<Overview>('overview', controller.signal, database)
      .then((data) => {
        setOverview(data)
        setError('')
        setMatches(null)
      })
      .catch((e) => {
        if (e.name !== 'AbortError') setError(e.message)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [generation, database])
  useEffect(() => {
    if (selected === null) return
    const controller = new AbortController()
    api<Detail>(`record?key=${encodeURIComponent(selected)}`, controller.signal, database)
      .then(setDetail)
      .catch((e) => {
        if (e.name !== 'AbortError') setDetailError(e.message)
      })
    return () => controller.abort()
  }, [selected, generation, database])

  const records = overview?.records ?? []
  const indexes = overview?.indexes ?? []
  const namespaces = [
    ...new Set(
      records.filter((r) => r.key.includes(':')).map((r) => r.key.slice(0, r.key.indexOf(':') + 1)),
    ),
  ].sort()
  const filtered = records.filter(
    (r) => r.key.startsWith(prefix) && r.key.toLowerCase().includes(search.toLowerCase()),
  )
  const pageCount = Math.max(1, Math.ceil(filtered.length / 50))
  const currentPage = Math.min(page, pageCount - 1)
  const visible = filtered.slice(currentPage * 50, (currentPage + 1) * 50)
  function openRecord(key: string) {
    if (key === selected) return
    setDetail(null)
    setDetailError('')
    setSelected(key)
  }
  function changeDatabase(name: string) {
    lookupController.current?.abort()
    setQuerying(false)
    setDatabase(name)
    setOverview(null)
    setSelected(null)
    setDetail(null)
    setDetailError('')
    setIndexName('')
    setMatches(null)
    setLookupError('')
    setSearch('')
    setPrefix('')
    setPage(0)
    setLoading(true)
  }
  function refresh() {
    setLoading(true)
    setDetail(null)
    setDetailError('')
    setGeneration((n) => n + 1)
  }
  async function lookup(e: FormEvent) {
    e.preventDefault()
    lookupController.current?.abort()
    const controller = new AbortController()
    lookupController.current = controller
    setLookupError('')
    setQuerying(true)
    setMatches(null)
    try {
      setMatches(
        await api<string[]>(
          `lookup?name=${encodeURIComponent(indexName || indexes[0]?.name || '')}&value=${encodeURIComponent(query)}`,
          controller.signal,
          database,
        ),
      )
    } catch (e) {
      if (!controller.signal.aborted)
        setLookupError(e instanceof Error ? e.message : 'Lookup failed')
    } finally {
      if (!controller.signal.aborted) setQuerying(false)
    }
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="/" aria-label="Dora Studio home">
          <span className="brand-mark">d</span>
          <strong>
            dora<span>studio</span>
          </strong>
        </a>
        <div className="workspace-label">WORKSPACE</div>
        <div className="database-card">
          <Icon kind="database" />
          <div>
            {databases.length > 1 ? (
              <select
                aria-label="Select database"
                value={database || databases[0]}
                onChange={(e) => changeDatabase(e.target.value)}
              >
                {databases.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            ) : (
              <strong>{overview?.name || 'Connecting...'}</strong>
            )}
            <span>Local database</span>
          </div>
          <span className="status-dot" />
        </div>
        <nav aria-label="Studio navigation">
          <button
            className={tab === 'records' ? 'nav-item active' : 'nav-item'}
            onClick={() => setTab('records')}
          >
            <Icon kind="grid" />
            Records<span>{records.length}</span>
          </button>
          <button
            className={tab === 'indexes' ? 'nav-item active' : 'nav-item'}
            onClick={() => setTab('indexes')}
          >
            <Icon kind="index" />
            Indexes<span>{indexes.length}</span>
          </button>
        </nav>
        <div className="namespace-heading">
          NAMESPACES <span>{namespaces.length}</span>
        </div>
        <button
          className={`namespace ${prefix === '' ? 'chosen' : ''}`}
          onClick={() => {
            setPrefix('')
            setPage(0)
            setTab('records')
          }}
        >
          <span className="namespace-icon">#</span>All keys<span>{records.length}</span>
        </button>
        {namespaces.map((ns) => (
          <button
            key={ns}
            className={`namespace ${prefix === ns ? 'chosen' : ''}`}
            onClick={() => {
              setPrefix(ns)
              setPage(0)
              setTab('records')
            }}
          >
            <span className="namespace-icon">#</span>
            {ns}
            <span>{records.filter((r) => r.key.startsWith(ns)).length}</span>
          </button>
        ))}
        <div className="sidebar-bottom">
          <Icon kind="lock" />
          <div>
            <strong>A window into your data</strong>
            <p>Explore freely. Studio is read-only.</p>
          </div>
        </div>
      </aside>
      <main>
        <header className="topbar">
          <div>
            <Icon kind="database" />
            <span>{overview?.name || 'Database'}</span>
            <span className="breadcrumb">/</span>
            <strong>{tab === 'records' ? 'Records' : 'Indexes'}</strong>
          </div>
          <span className="read-only">
            <Icon kind="lock" />
            Read only
          </span>
        </header>
        <div className="page-content">
          <div className="page-heading">
            <div>
              <div className="eyebrow">DORA DATABASE EXPLORER</div>
              <h1>{tab === 'records' ? 'Your data, in view.' : 'Find the right records.'}</h1>
              <p>
                {tab === 'records'
                  ? 'Browse your keyspace. Follow connections. Get the full picture.'
                  : 'Inspect index definitions and look up exact field values.'}
              </p>
            </div>
            <button className="button refresh" onClick={refresh} disabled={loading}>
              <Icon kind="refresh" />
              {loading ? 'Reading...' : 'Refresh'}
            </button>
          </div>
          {error && (
            <div className="error" role="alert">
              {error}
              <button onClick={refresh}>Retry</button>
            </div>
          )}
          <section className="stats" aria-label="Database statistics">
            <div>
              <span>Total records</span>
              <strong>{overview ? records.length.toLocaleString() : '-'}</strong>
              <small>Across your keyspace</small>
            </div>
            <div>
              <span>Secondary indexes</span>
              <strong>{overview ? indexes.length : '-'}</strong>
              <small>Exact-match lookups</small>
            </div>
            <div>
              <span>Sequence</span>
              <strong>{overview?.seq || '-'}</strong>
              <small>Position in operation history</small>
            </div>
            <div>
              <span>Connection</span>
              <strong className="connection">
                <span className="status-dot" />
                {overview?.mode === 'live' ? 'Live' : 'Offline view'}
              </strong>
              <small>
                {overview?.mode === 'live'
                  ? 'Connected to your application'
                  : 'Application must be stopped'}
              </small>
            </div>
          </section>
          {tab === 'records' ? (
            <section className="data-panel">
              <div className="panel-heading">
                <div>
                  <h2>
                    Records <span>{filtered.length}</span>
                  </h2>
                  <p>{prefix ? `Keys starting with ${prefix}` : 'All keys in this database'}</p>
                </div>
                <label className="search">
                  <Icon kind="search" />
                  <input
                    aria-label="Search keys"
                    placeholder="Search keys..."
                    value={search}
                    onChange={(e) => {
                      setSearch(e.target.value)
                      setPage(0)
                    }}
                  />
                </label>
              </div>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>KEY</th>
                      <th>TYPE</th>
                      <th>VALUE PREVIEW</th>
                      <th>
                        <span className="sr-only">Open record</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {visible.map((record) => (
                      <tr
                        key={record.key}
                        className={record.key === selected ? 'selected-row' : ''}
                      >
                        <td>
                          <button className="key-button" onClick={() => openRecord(record.key)}>
                            <span className="key-hash">#</span>
                            {record.key}
                          </button>
                        </td>
                        <td>
                          <span className={`type-badge ${record.type.toLowerCase()}`}>
                            {record.type}
                          </span>
                        </td>
                        <td className="preview">{record.preview}</td>
                        <td>
                          <button
                            className="icon-button"
                            aria-label={`Inspect ${record.key}`}
                            onClick={() => openRecord(record.key)}
                          >
                            <Icon kind="arrow" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {!visible.length && (
                <div className="empty">
                  <Icon kind="database" />
                  <h3>
                    {loading
                      ? 'Reading your database...'
                      : records.length
                        ? 'No matching keys'
                        : 'Your keyspace is empty'}
                  </h3>
                  <p>
                    {records.length
                      ? 'Try another search or namespace.'
                      : 'Records will appear here when your application stores data.'}
                  </p>
                </div>
              )}
              <div className="table-footer">
                <span>
                  {filtered.length
                    ? `${currentPage * 50 + 1}-${Math.min((currentPage + 1) * 50, filtered.length)} of ${filtered.length} records`
                    : '0 records'}
                </span>
                <div>
                  <button disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>
                    Previous
                  </button>
                  <span>
                    {currentPage + 1} / {pageCount}
                  </span>
                  <button
                    disabled={currentPage + 1 >= pageCount}
                    onClick={() => setPage(currentPage + 1)}
                  >
                    Next
                  </button>
                </div>
              </div>
            </section>
          ) : (
            <section className="data-panel index-panel">
              <div className="panel-heading">
                <div>
                  <h2>
                    Index definitions <span>{indexes.length}</span>
                  </h2>
                  <p>Non-unique indexes on top-level object fields</p>
                </div>
              </div>
              {indexes.length ? (
                <>
                  <div className="index-list">
                    {indexes.map((idx) => (
                      <button
                        className={`index-card ${(indexName || indexes[0]?.name) === idx.name ? 'selected-index' : ''}`}
                        key={idx.name}
                        onClick={() => {
                          setIndexName(idx.name)
                          setMatches(null)
                          setLookupError('')
                        }}
                      >
                        <Icon kind="index" />
                        <strong>{idx.name}</strong>
                        <span>
                          Prefix <code>{idx.prefix || '(all keys)'}</code>
                        </span>
                        <span>
                          Field <code>{idx.field}</code>
                        </span>
                      </button>
                    ))}
                  </div>
                  <form className="lookup-form" onSubmit={lookup}>
                    <label htmlFor="lookup-value">
                      Look up a value in <strong>{indexName || indexes[0]?.name}</strong>
                    </label>
                    <p>
                      Enter a JSON scalar: a quoted string, number, boolean, null, or encoded
                      reference.
                    </p>
                    <div>
                      <input
                        id="lookup-value"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        required
                      />
                      <button className="button primary" disabled={querying}>
                        {querying ? 'Looking up...' : 'Run lookup'}
                        <Icon kind="arrow" />
                      </button>
                    </div>
                  </form>
                  {lookupError && (
                    <div className="error" role="alert">
                      {lookupError}
                    </div>
                  )}
                  {matches !== null && (
                    <div className="lookup-results">
                      <h3>
                        {matches.length} matching {matches.length === 1 ? 'record' : 'records'}
                      </h3>
                      {matches.map((key) => (
                        <button className="result-link" key={key} onClick={() => openRecord(key)}>
                          {key}
                          <Icon kind="arrow" />
                        </button>
                      ))}
                      {!matches.length && <p>No records have this indexed value.</p>}
                    </div>
                  )}
                </>
              ) : (
                <div className="empty">
                  <Icon kind="index" />
                  <h3>No indexes yet</h3>
                  <p>Create an index in your application with db.EnsureIndex().</p>
                </div>
              )}
            </section>
          )}
          <footer className="page-footer">
            <span>
              <span className="status-dot" /> Local /{' '}
              {overview?.mode === 'live' ? 'Live inspection' : 'Files opened read-only'}
            </span>
            <span title={overview?.hash}>
              STATE HASH <code>{overview?.hash.slice(0, 16) || '-'}...</code>
            </span>
          </footer>
        </div>
      </main>
      {selected !== null && (
        <aside className="inspector" aria-label="Record inspector">
          <div className="inspector-header">
            <span>RECORD INSPECTOR</span>
            <button
              className="icon-button"
              aria-label="Close inspector"
              onClick={() => setSelected(null)}
            >
              Close
            </button>
          </div>
          <h2>{selected}</h2>
          <div className="inspector-meta">
            <span className="type-badge">{detail?.type || 'Loading'}</span>
            <span>
              <Icon kind="lock" />
              Read only
            </span>
          </div>
          <div className="value-heading">
            VALUE <span>JSON</span>
          </div>
          {detailError ? (
            <div className="error" role="alert">
              {detailError}
            </div>
          ) : detail ? (
            <pre className="json-view">
              <JsonValue value={detail.value} onReference={openRecord} />
            </pre>
          ) : (
            <p className="inspector-loading">Reading record...</p>
          )}
          {detail && detail.references.length > 0 && (
            <div className="references">
              <h3>
                References <span>{detail.references.length}</span>
              </h3>
              {detail.references.map((key) => (
                <button className="result-link" key={key} onClick={() => openRecord(key)}>
                  {key}
                  <Icon kind="arrow" />
                </button>
              ))}
            </div>
          )}
          <div className="inspector-note">
            Values are displayed as stored. Following a reference opens its target record.
          </div>
        </aside>
      )}
    </div>
  )
}
