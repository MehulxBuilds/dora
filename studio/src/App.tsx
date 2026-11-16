import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import {
  ArrowClockwiseIcon,
  ArrowUpRightIcon,
  CaretLeftIcon,
  CaretRightIcon,
  DatabaseIcon,
  FingerprintIcon,
  HashIcon,
  ListBulletsIcon,
  LockSimpleIcon,
  MagnifyingGlassIcon,
  SquaresFourIcon,
  StackIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Skeleton } from '@/components/ui/skeleton'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { RecordInspector, isPlainNavigation } from '@/components/studio/record-inspector'
import { useNavigation } from '@/lib/navigation'
import { readStudio, useStudioResource } from '@/lib/studio-api'
import type { Overview } from '@/lib/studio-api'
import { cn } from '@/lib/utils'

const numbers = new Intl.NumberFormat()

export default function App() {
  const { params, navigate, recordHref } = useNavigation()
  const database = params.get('db') ?? ''
  const tab = params.get('view') === 'indexes' ? 'indexes' : 'records'
  const search = params.get('search') ?? ''
  const prefix = params.get('prefix') ?? ''
  const selected = params.get('key')
  const indexName = params.get('index') ?? ''
  const pageValue = Number(params.get('page') ?? 0)
  const page = Number.isSafeInteger(pageValue) && pageValue > 0 ? pageValue : 0
  const [generation, setGeneration] = useState(0)
  const overview = useStudioResource<Overview>('overview', database, generation)
  const catalog = useStudioResource<string[]>('databases')
  const [query, setQuery] = useState('"mehul@example.com"')
  const [lookup, setLookup] = useState<{ scope: string; keys?: string[]; error?: string } | null>(
    null,
  )
  const [pending, setPending] = useState<string | null>(null)
  const controller = useRef<AbortController | null>(null)
  const queryInput = useRef<HTMLInputElement>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const records = overview.data?.records ?? []
  const indexes = overview.data?.indexes ?? []
  const activeIndex = indexes.find((index) => index.name === indexName) ?? indexes[0]
  const scope = JSON.stringify([database, activeIndex?.name, generation])
  const result = lookup?.scope === scope ? lookup : null
  const namespaces = [
    ...new Set(
      records
        .filter((record) => record.key.includes(':'))
        .map((record) => record.key.slice(0, record.key.indexOf(':') + 1)),
    ),
  ].sort()
  const filtered = records.filter(
    (record) =>
      record.key.startsWith(prefix) && record.key.toLowerCase().includes(search.toLowerCase()),
  )
  const pages = Math.max(1, Math.ceil(filtered.length / 50))
  const currentPage = Math.min(page, pages - 1)
  const visible = filtered.slice(currentPage * 50, (currentPage + 1) * 50)
  const loading = overview.loading
  const error = overview.error ?? catalog.error
  const name = overview.data?.name || database || catalog.data?.[0] || 'Database'
  function openRecord(key: string) {
    navigate({ key }, true)
  }
  function refresh() {
    controller.current?.abort()
    setPending(null)
    setGeneration((value) => value + 1)
  }
  function switchDatabase(value: string) {
    controller.current?.abort()
    setPending(null)
    navigate({ db: value, key: null, prefix: null, search: null, page: null, index: null }, true)
  }
  async function runLookup(event: FormEvent) {
    event.preventDefault()
    if (!activeIndex) return
    controller.current?.abort()
    const request = new AbortController()
    controller.current = request
    setLookup(null)
    setPending(scope)
    try {
      const keys = await readStudio<string[]>(
        `lookup?name=${encodeURIComponent(activeIndex.name)}&value=${encodeURIComponent(query)}`,
        database,
        request.signal,
      )
      if (!request.signal.aborted) setLookup({ scope, keys })
    } catch (error) {
      if (!request.signal.aborted) {
        setLookup({ scope, error: error instanceof Error ? error.message : 'Lookup failed.' })
        queryInput.current?.focus()
      }
    } finally {
      if (!request.signal.aborted) setPending(null)
    }
  }
  const databaseSelect = (
    <NativeSelect
      className="w-full [&>select]:h-9 [&>select]:rounded-md [&>select]:bg-card [&>select]:text-foreground"
      name="database"
      aria-label="Select database"
      value={database || catalog.data?.[0] || ''}
      onChange={(event) => switchDatabase(event.target.value)}
      disabled={!catalog.data?.length}
    >
      {(catalog.data ?? ['']).map((item) => (
        <NativeSelectOption key={item} value={item}>
          {item || 'Connecting?'}
        </NativeSelectOption>
      ))}
    </NativeSelect>
  )

  return (
    <div className="flex h-dvh flex-col overflow-hidden bg-background font-sans text-foreground antialiased">
      <a
        href="#main"
        className="sr-only z-[100] rounded-md bg-primary p-3 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
      >
        Skip to Content
      </a>
      <header className="relative z-10 flex h-16 shrink-0 items-center justify-between gap-4 border-b bg-background px-5 lg:px-7">
        <div className="flex min-w-0 items-center gap-5">
          <a
            href="/"
            className="flex shrink-0 items-center gap-2.5 rounded-sm focus-visible:outline-2 focus-visible:outline-offset-4"
            aria-label="Dora Studio home"
          >
            <span className="flex size-7 items-center justify-center rounded-md border border-white/20 bg-white text-lg font-semibold text-black">
              d
            </span>
            <span className="text-[15px] font-semibold tracking-tight" translate="no">
              dora<span className="ml-1 font-normal text-muted-foreground">studio</span>
            </span>
          </a>
          <span className="hidden text-2xl font-light text-neutral-700 sm:block" aria-hidden="true">
            /
          </span>
          <span className="hidden min-w-0 items-center gap-2 text-xs sm:flex">
            <DatabaseIcon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="truncate" translate="no">
              {name}
            </span>
            <Badge
              variant="outline"
              className="ml-1 rounded-full px-2 text-[10px] font-normal text-muted-foreground"
            >
              Local
            </Badge>
          </span>
        </div>
        <span className="flex shrink-0 items-center gap-2 text-[11px] text-muted-foreground">
          <LockSimpleIcon className="size-3.5" aria-hidden="true" />
          Read only
        </span>
      </header>
      <div className="mx-auto flex min-h-0 w-full max-w-[1680px] flex-1 overflow-hidden">
        <aside
          className="hidden h-full min-h-0 w-56 shrink-0 flex-col overflow-hidden border-r px-4 py-7 lg:flex"
          aria-label="Database navigation"
        >
          <p className="mb-3 px-2 text-[10px] font-medium tracking-widest text-muted-foreground">
            WORKSPACE
          </p>
          {databaseSelect}
          <div className="mt-8 mb-3 flex items-center justify-between px-2">
            <p className="text-[10px] font-medium tracking-widest text-muted-foreground">
              NAMESPACES
            </p>
            <span className="text-[10px] tabular-nums text-muted-foreground">
              {namespaces.length}
            </span>
          </div>
          <nav className="min-h-0 space-y-1 overflow-y-auto" aria-label="Namespace filters">
            <Button
              variant="ghost"
              className={cn(
                'h-9 w-full justify-start gap-2.5 rounded-md px-2 text-xs font-normal text-muted-foreground',
                !prefix && 'bg-accent text-foreground',
              )}
              onClick={() => navigate({ prefix: null, page: null, view: null })}
              aria-current={!prefix ? 'page' : undefined}
            >
              <SquaresFourIcon className="size-3.5" aria-hidden="true" />
              All Records
              <span className="ml-auto text-[10px] tabular-nums">
                {numbers.format(records.length)}
              </span>
            </Button>
            {namespaces.map((namespace) => (
              <Button
                key={namespace}
                variant="ghost"
                className={cn(
                  'h-9 w-full justify-start gap-2.5 rounded-md px-2 text-xs font-normal text-muted-foreground',
                  prefix === namespace && 'bg-accent text-foreground',
                )}
                onClick={() => navigate({ prefix: namespace, page: null, view: null })}
                aria-current={prefix === namespace ? 'page' : undefined}
              >
                <HashIcon className="size-3.5 shrink-0" aria-hidden="true" />
                <span className="truncate" translate="no">
                  {namespace}
                </span>
                <span className="ml-auto text-[10px] tabular-nums">
                  {numbers.format(
                    records.filter((record) => record.key.startsWith(namespace)).length,
                  )}
                </span>
              </Button>
            ))}
          </nav>
          <div className="mt-auto pt-8">
            <div className="rounded-lg border border-dashed p-3.5">
              <LockSimpleIcon className="mb-3 size-4 text-muted-foreground" aria-hidden="true" />
              <p className="text-xs font-medium">Your data. Just in view.</p>
              <p className="mt-2 text-[11px] leading-5 text-muted-foreground">
                Explore records freely. Studio never writes to your database.
              </p>
            </div>
            <p className="mt-4 flex items-center gap-2 px-1 text-[10px] text-muted-foreground">
              <TerminalWindowIcon className="size-3.5" aria-hidden="true" />
              Embedded. Local. Yours.
            </p>
          </div>
        </aside>
        <main
          id="main"
          tabIndex={-1}
          className="h-full min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-4 py-7 outline-none sm:px-8 lg:px-10 lg:py-10 xl:px-12"
        >
          <div className="mb-6 grid grid-cols-2 gap-3 lg:hidden">
            {databaseSelect}
            <NativeSelect
              className="w-full [&>select]:h-9 [&>select]:rounded-md [&>select]:bg-card [&>select]:text-foreground"
              name="namespace"
              aria-label="Filter namespace"
              value={prefix}
              onChange={(event) => navigate({ prefix: event.target.value, page: null, view: null })}
            >
              <NativeSelectOption value="">All Namespaces</NativeSelectOption>
              {namespaces.map((namespace) => (
                <NativeSelectOption key={namespace} value={namespace}>
                  {namespace}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
          <div className="mb-8 flex items-start justify-between gap-4">
            <div>
              <p className="mb-3 flex items-center gap-2 text-[11px] text-muted-foreground">
                <DatabaseIcon className="size-3.5" aria-hidden="true" />
                Workspace<span className="px-1 text-neutral-600">/</span>
                <span className="text-foreground">Database</span>
              </p>
              <h1 className="text-balance text-[28px] font-semibold tracking-[-0.9px] sm:text-[30px]">
                Database Explorer
              </h1>
              <p className="mt-2 max-w-lg text-pretty text-[13px] leading-6 text-muted-foreground">
                Browse your keyspace, follow references, and inspect indexes.
              </p>
            </div>
            <Button
              variant="outline"
              className="mt-1 h-9 gap-2 rounded-md bg-card px-3 text-xs"
              onClick={refresh}
              disabled={loading}
              aria-label="Refresh database"
            >
              <ArrowClockwiseIcon
                className={cn('size-3.5', loading && 'animate-spin motion-reduce:animate-none')}
                aria-hidden="true"
              />
              <span className="hidden sm:inline">{loading ? 'Reading?' : 'Refresh'}</span>
            </Button>
          </div>
          {error && (
            <Alert
              variant="destructive"
              className="mb-6 rounded-md border-red-900/60 bg-red-950/20 p-4"
            >
              <AlertTitle>Database Connection Unavailable</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <section
            className="mb-8 grid grid-cols-1 overflow-hidden rounded-lg border bg-card min-[450px]:grid-cols-3"
            aria-label="Database statistics"
          >
            {[
              {
                label: 'Total Records',
                value: records.length,
                note: 'Across your keyspace',
                icon: DatabaseIcon,
              },
              {
                label: 'Secondary Indexes',
                value: indexes.length,
                note: 'Exact-match lookups',
                icon: StackIcon,
              },
              {
                label: 'Sequence Number',
                value: overview.data?.seq,
                note: 'Position in operation history',
                icon: FingerprintIcon,
              },
            ].map(({ label, value, note, icon: StatIcon }) => (
              <div
                key={label}
                className="border-b p-5 last:border-0 min-[450px]:border-r min-[450px]:border-b-0 sm:p-6"
              >
                <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <h2>{label}</h2>
                  <StatIcon className="size-4 shrink-0" aria-hidden="true" />
                </div>
                {loading ? (
                  <Skeleton className="my-3 h-8 w-16 rounded-sm motion-reduce:animate-none" />
                ) : (
                  <p className="mt-3 mb-2 text-[28px] leading-8 font-medium tracking-tight tabular-nums">
                    {error || value === undefined
                      ? '?'
                      : typeof value === 'number'
                        ? numbers.format(value)
                        : numbers.format(BigInt(value))}
                  </p>
                )}
                <p className="text-[11px] text-muted-foreground">{note}</p>
              </div>
            ))}
          </section>
          <Tabs
            value={tab}
            onValueChange={(value) => navigate({ view: value === 'indexes' ? 'indexes' : null })}
            className="gap-0"
          >
            <div className="mb-5 flex items-center justify-between gap-3 border-b">
              <TabsList variant="line" className="h-11 gap-6 p-0" aria-label="Database views">
                <TabsTrigger value="records" className="h-11 gap-2 px-0 text-xs after:bottom-0">
                  <SquaresFourIcon className="size-3.5" aria-hidden="true" />
                  Records
                  <Badge
                    variant="secondary"
                    className="h-4 rounded-sm px-1.5 text-[10px] font-normal tabular-nums"
                  >
                    {records.length}
                  </Badge>
                </TabsTrigger>
                <TabsTrigger value="indexes" className="h-11 gap-2 px-0 text-xs after:bottom-0">
                  <ListBulletsIcon className="size-3.5" aria-hidden="true" />
                  Indexes
                  <Badge
                    variant="secondary"
                    className="h-4 rounded-sm px-1.5 text-[10px] font-normal tabular-nums"
                  >
                    {indexes.length}
                  </Badge>
                </TabsTrigger>
              </TabsList>
              <span className="hidden text-[10px] text-muted-foreground sm:inline">
                {overview.data?.mode === 'live' ? 'Live connection' : 'Offline inspection'}
              </span>
            </div>
            <TabsContent value="records">
              <section className="overflow-hidden rounded-lg border bg-card" aria-label="Records">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 sm:px-5">
                  <div className="flex items-center gap-2 text-xs">
                    <span className="font-medium">{prefix || 'All Records'}</span>
                    <span className="text-neutral-600">/</span>
                    <span className="tabular-nums text-muted-foreground">
                      {numbers.format(filtered.length)} results
                    </span>
                  </div>
                  <div className="relative w-full sm:w-60">
                    <MagnifyingGlassIcon
                      className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
                      aria-hidden="true"
                    />
                    <Input
                      name="search"
                      aria-label="Search keys"
                      autoComplete="off"
                      spellCheck={false}
                      placeholder="Search keys?"
                      className="h-9 rounded-md bg-background pl-9 text-xs"
                      value={search}
                      onChange={(event) => navigate({ search: event.target.value, page: null })}
                    />
                  </div>
                </div>
                <Table className="min-w-[540px] table-fixed text-xs">
                  <TableHeader>
                    <TableRow className="h-10 bg-muted/30 hover:bg-muted/30">
                      <TableHead className="w-[30%] px-5 text-[11px] text-muted-foreground">
                        Key
                      </TableHead>
                      <TableHead className="w-[17%] px-3 text-[11px] text-muted-foreground">
                        Type
                      </TableHead>
                      <TableHead className="px-3 text-[11px] text-muted-foreground">
                        Value Preview
                      </TableHead>
                      <TableHead className="w-11">
                        <span className="sr-only">Inspect</span>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {loading
                      ? Array.from({ length: 5 }, (_, index) => (
                          <TableRow key={`loading-${index}`} className="h-14">
                            <TableCell className="px-5">
                              <Skeleton className="h-3 w-24 motion-reduce:animate-none" />
                            </TableCell>
                            <TableCell>
                              <Skeleton className="h-4 w-12 motion-reduce:animate-none" />
                            </TableCell>
                            <TableCell colSpan={2}>
                              <Skeleton className="h-3 w-3/4 motion-reduce:animate-none" />
                            </TableCell>
                          </TableRow>
                        ))
                      : visible.map((record) => (
                          <TableRow
                            key={record.key}
                            className="h-14"
                            data-state={selected === record.key ? 'selected' : undefined}
                          >
                            <TableCell className="px-5">
                              <a
                                href={recordHref(record.key)}
                                onClick={(event) => {
                                  if (isPlainNavigation(event)) {
                                    event.preventDefault()
                                    openRecord(record.key)
                                  }
                                }}
                                className="flex min-w-0 items-center gap-2 rounded-sm py-2 hover:underline hover:underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2"
                                translate="no"
                              >
                                <HashIcon
                                  className="size-3.5 shrink-0 text-muted-foreground"
                                  aria-hidden="true"
                                />
                                <span className="truncate" title={record.key}>
                                  {record.key}
                                </span>
                              </a>
                            </TableCell>
                            <TableCell className="px-3">
                              <Badge
                                variant="outline"
                                className="h-5 rounded-sm px-1.5 text-[10px] font-normal text-muted-foreground"
                              >
                                {record.type}
                              </Badge>
                            </TableCell>
                            <TableCell
                              className="truncate px-3 text-[11px] text-muted-foreground"
                              title={record.preview}
                              translate="no"
                            >
                              {record.preview}
                            </TableCell>
                            <TableCell>
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                className="rounded-md text-muted-foreground"
                                aria-label={`Inspect ${record.key}`}
                                onClick={() => openRecord(record.key)}
                              >
                                <ArrowUpRightIcon className="size-3.5" aria-hidden="true" />
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                  </TableBody>
                </Table>
                {!loading && !visible.length && (
                  <div className="flex min-h-60 flex-col items-center justify-center gap-3 px-6 text-center">
                    <div className="flex size-11 items-center justify-center rounded-lg border bg-muted/30">
                      <DatabaseIcon className="size-5 text-muted-foreground" aria-hidden="true" />
                    </div>
                    <h3 className="text-sm font-medium">
                      {error
                        ? 'Records Unavailable'
                        : records.length
                          ? 'No Matching Records'
                          : 'No Records Yet'}
                    </h3>
                    <p className="max-w-xs text-xs leading-5 text-muted-foreground">
                      {error
                        ? 'Connect the local server, then refresh to try again.'
                        : records.length
                          ? 'Try another search or namespace.'
                          : 'Store a value in your application, then refresh to see it here.'}
                    </p>
                    {(search || prefix) && (
                      <Button
                        variant="outline"
                        className="mt-1 rounded-md"
                        onClick={() => navigate({ search: null, prefix: null, page: null })}
                      >
                        Clear Filters
                      </Button>
                    )}
                  </div>
                )}
                <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 text-[11px] text-muted-foreground sm:px-5">
                  <span aria-live="polite" className="tabular-nums">
                    {filtered.length
                      ? `${visible.length === 1 ? numbers.format(currentPage * 50 + 1) : `${numbers.format(currentPage * 50 + 1)}\u2013${numbers.format(Math.min((currentPage + 1) * 50, filtered.length))}`} of ${numbers.format(filtered.length)} ${filtered.length === 1 ? 'record' : 'records'}`
                      : '0 records'}
                  </span>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="icon-sm"
                      className="rounded-md"
                      aria-label="Previous page"
                      disabled={currentPage === 0 || loading}
                      onClick={() => navigate({ page: String(currentPage - 1) })}
                    >
                      <CaretLeftIcon className="size-3" aria-hidden="true" />
                    </Button>
                    <span className="min-w-12 text-center tabular-nums">
                      {currentPage + 1} / {pages}
                    </span>
                    <Button
                      variant="outline"
                      size="icon-sm"
                      className="rounded-md"
                      aria-label="Next page"
                      disabled={currentPage + 1 >= pages || loading}
                      onClick={() => navigate({ page: String(currentPage + 1) })}
                    >
                      <CaretRightIcon className="size-3" aria-hidden="true" />
                    </Button>
                  </div>
                </div>
              </section>
            </TabsContent>
            <TabsContent value="indexes">
              <section
                className="overflow-hidden rounded-lg border bg-card"
                aria-label="Index definitions"
              >
                <div className="border-b p-5">
                  <h2 className="text-sm font-medium">Secondary Indexes</h2>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    Non-unique indexes on top-level object fields.
                  </p>
                </div>
                {loading ? (
                  <div className="p-6">
                    <Skeleton className="h-20 w-full rounded-md motion-reduce:animate-none" />
                  </div>
                ) : indexes.length ? (
                  <>
                    <div className="grid gap-3 p-5 sm:grid-cols-2 xl:grid-cols-3">
                      {indexes.map((index) => (
                        <Button
                          key={index.name}
                          variant="outline"
                          className={cn(
                            'h-auto min-w-0 flex-col items-start gap-3 rounded-md p-4 text-left font-normal whitespace-normal',
                            activeIndex?.name === index.name && 'border-foreground/50 bg-accent',
                          )}
                          onClick={() => navigate({ index: index.name })}
                          aria-pressed={activeIndex?.name === index.name}
                        >
                          <div className="flex w-full min-w-0 items-center gap-2">
                            <ListBulletsIcon
                              className="size-4 shrink-0 text-muted-foreground"
                              aria-hidden="true"
                            />
                            <span className="truncate text-xs font-medium" translate="no">
                              {index.name}
                            </span>
                          </div>
                          <div className="flex flex-wrap gap-3 text-[11px] text-muted-foreground">
                            <span>
                              Prefix:{' '}
                              <span className="text-foreground" translate="no">
                                {index.prefix || 'All keys'}
                              </span>
                            </span>
                            <span>
                              Field:{' '}
                              <span className="text-foreground" translate="no">
                                {index.field}
                              </span>
                            </span>
                          </div>
                        </Button>
                      ))}
                    </div>
                    <form className="border-t p-5 sm:p-6" onSubmit={runLookup}>
                      <label htmlFor="lookup-value" className="text-sm font-medium">
                        Look Up a Value
                      </label>
                      <p
                        id="lookup-hint"
                        className="mt-2 mb-4 text-xs leading-5 text-muted-foreground"
                      >
                        Search {activeIndex?.name} with a JSON string, number, boolean, null, or
                        encoded reference.
                      </p>
                      <div className="flex flex-col gap-2 sm:flex-row">
                        <Input
                          ref={queryInput}
                          id="lookup-value"
                          name="lookup-value"
                          autoComplete="off"
                          spellCheck={false}
                          className="h-9 rounded-md bg-background text-xs"
                          value={query}
                          onChange={(event) => setQuery(event.target.value)}
                          required
                          aria-invalid={!!result?.error}
                          aria-describedby={
                            result?.error ? 'lookup-hint lookup-error' : 'lookup-hint'
                          }
                        />
                        <Button
                          type="submit"
                          className="h-9 gap-2 rounded-md px-4 text-xs"
                          disabled={pending === scope}
                        >
                          <MagnifyingGlassIcon className="size-3.5" aria-hidden="true" />
                          {pending === scope ? 'Looking Up?' : 'Run Lookup'}
                        </Button>
                      </div>
                      {result?.error && (
                        <p id="lookup-error" role="alert" className="mt-3 text-xs text-red-400">
                          {result.error} Check the JSON value and try again.
                        </p>
                      )}
                      <div aria-live="polite">
                        {result?.keys && (
                          <div className="mt-5 overflow-hidden rounded-md border">
                            <h3 className="border-b bg-muted/30 px-4 py-3 text-xs font-medium">
                              {numbers.format(result.keys.length)} Matching Records
                            </h3>
                            {result.keys.slice(0, 50).map((key) => (
                              <a
                                key={key}
                                href={recordHref(key)}
                                onClick={(event) => {
                                  if (isPlainNavigation(event)) {
                                    event.preventDefault()
                                    openRecord(key)
                                  }
                                }}
                                className="flex items-center justify-between gap-3 border-b px-4 py-3 text-xs last:border-0 hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-[-2px]"
                              >
                                <span className="truncate" translate="no">
                                  {key}
                                </span>
                                <ArrowUpRightIcon
                                  className="size-3.5 shrink-0"
                                  aria-hidden="true"
                                />
                              </a>
                            ))}
                            {!result.keys.length && (
                              <p className="p-4 text-xs text-muted-foreground">
                                No records match this value.
                              </p>
                            )}
                            {result.keys.length > 50 && (
                              <p className="p-4 text-xs text-muted-foreground">
                                Showing the first 50 matches.
                              </p>
                            )}
                          </div>
                        )}
                      </div>
                    </form>
                  </>
                ) : (
                  <div className="flex min-h-64 flex-col items-center justify-center gap-3 px-6 text-center">
                    <StackIcon className="size-7 text-muted-foreground" aria-hidden="true" />
                    <h3 className="text-sm font-medium">
                      {error ? 'Indexes Unavailable' : 'No Indexes Yet'}
                    </h3>
                    <p className="max-w-sm text-xs leading-6 text-muted-foreground">
                      {error
                        ? 'Connect the local server, then refresh to try again.'
                        : 'Create an index with db.EnsureIndex() in your application, then refresh to inspect it here.'}
                    </p>
                  </div>
                )}
              </section>
            </TabsContent>
          </Tabs>
          <footer className="mt-6 flex flex-wrap justify-between gap-3 text-[10px] text-muted-foreground">
            <span className="flex items-center gap-2">
              <span
                className={cn('size-1.5 rounded-full', error ? 'bg-neutral-500' : 'bg-emerald-400')}
                aria-hidden="true"
              />
              {error
                ? 'Disconnected'
                : overview.data?.mode === 'live'
                  ? 'Live connection'
                  : 'Offline inspection'}
              <span className="text-neutral-700">/</span>Local access only
            </span>
            <span className="flex items-center gap-2">
              <FingerprintIcon className="size-3" aria-hidden="true" />
              State Hash
              <span className="select-all text-foreground/70" title={overview.data?.hash}>
                {overview.data ? `${overview.data.hash.slice(0, 16)}?` : '?'}
              </span>
            </span>
          </footer>
        </main>
      </div>
      <RecordInspector
        selected={selected}
        database={database}
        generation={generation}
        navigation={{ href: recordHref, open: openRecord }}
        close={() => navigate({ key: null })}
      />
    </div>
  )
}
