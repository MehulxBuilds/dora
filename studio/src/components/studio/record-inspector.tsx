import type { MouseEvent, ReactNode } from 'react'
import { ArrowUpRightIcon, DatabaseIcon, LockSimpleIcon } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { useStudioResource } from '@/lib/studio-api'
import type { RecordDetail } from '@/lib/studio-api'

export function isPlainNavigation(event: MouseEvent<HTMLAnchorElement>) {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey
}

type Navigation = { href: (key: string) => string; open: (key: string) => void }

function RecordLink({ target, navigation }: { target: string; navigation: Navigation }) {
  return (
    <a
      href={navigation.href(target)}
      onClick={(event) => {
        if (isPlainNavigation(event)) {
          event.preventDefault()
          navigation.open(target)
        }
      }}
      className="inline-flex max-w-full items-center gap-1 rounded-sm text-foreground underline decoration-muted-foreground underline-offset-4 hover:decoration-foreground focus-visible:outline-2 focus-visible:outline-offset-4"
      aria-label={`Open reference ${target}`}
    >
      <span className="break-all">{target}</span>
      <ArrowUpRightIcon className="size-3 shrink-0" aria-hidden="true" />
    </a>
  )
}

function JsonValue({
  value,
  navigation,
  depth = 0,
}: {
  value: unknown
  navigation: Navigation
  depth?: number
}): ReactNode {
  if (depth > 24) return <span className="break-all">{JSON.stringify(value)}</span>
  if (value === null) return <span className="text-muted-foreground">null</span>
  if (typeof value === 'string')
    return <span className="text-muted-foreground">{JSON.stringify(value)}</span>
  if (typeof value !== 'object')
    return <span className="text-muted-foreground">{String(value)}</span>
  const object = value as Record<string, unknown>
  if (
    !Array.isArray(value) &&
    Object.keys(object).length === 1 &&
    typeof object['\u0000_ref'] === 'string'
  ) {
    return <RecordLink target={object['\u0000_ref']} navigation={navigation} />
  }
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
      <span className="block border-l border-border pl-5 ml-0.5">
        {entries.map(([key, item], index) => (
          <span className="block" key={key}>
            {!array && (
              <>
                <span className="font-medium text-foreground">{JSON.stringify(key)}</span>:{' '}
              </>
            )}
            <JsonValue value={item} navigation={navigation} depth={depth + 1} />
            {index < entries.length - 1 ? ',' : ''}
          </span>
        ))}
      </span>
      {array ? ']' : '}'}
    </span>
  )
}

export function RecordInspector({
  selected,
  database,
  generation,
  navigation,
  close,
}: {
  selected: string | null
  database: string
  generation: number
  navigation: Navigation
  close: () => void
}) {
  const detail = useStudioResource<RecordDetail>(
    selected === null ? null : `record?key=${encodeURIComponent(selected)}`,
    database,
    generation,
  )
  return (
    <Sheet
      open={selected !== null}
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <SheetContent className="w-full gap-0 overflow-y-auto overscroll-contain border-l p-0 shadow-xl transition-[transform,opacity] motion-reduce:transition-none sm:w-[480px] sm:max-w-[480px] [&>button]:right-5 [&>button]:top-5 [&>button]:rounded-md [&>button]:focus-visible:ring-2">
        <SheetHeader className="gap-4 border-b px-6 py-6 pr-14">
          <span className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <DatabaseIcon className="size-4" aria-hidden="true" />
            Record Inspector
          </span>
          <SheetTitle className="break-all text-xl font-semibold tracking-tight" translate="no">
            {selected}
          </SheetTitle>
          <SheetDescription>Inspect the stored value and follow its references.</SheetDescription>
        </SheetHeader>
        <div className="flex items-center justify-between border-b px-6 py-4">
          <Badge variant="secondary" className="rounded-sm font-normal">
            {detail.data?.type ?? 'Record'}
          </Badge>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <LockSimpleIcon className="size-3" aria-hidden="true" />
            Read only
          </span>
        </div>
        <div className="p-6" aria-busy={detail.loading}>
          <div className="mb-4 flex items-center justify-between">
            <h3 className="text-sm font-medium">Value</h3>
            <span className="text-xs text-muted-foreground">JSON</span>
          </div>
          {detail.error ? (
            <Alert variant="destructive" className="rounded-md">
              <AlertDescription>
                {detail.error} Refresh the database or choose another record.
              </AlertDescription>
            </Alert>
          ) : detail.data ? (
            <pre
              className="overflow-x-auto rounded-md border bg-muted/30 p-5 font-sans text-[13px] leading-7 whitespace-pre-wrap break-words"
              translate="no"
            >
              <JsonValue value={detail.data.value} navigation={navigation} />
            </pre>
          ) : (
            <div className="space-y-3 rounded-md border p-5" aria-label="Loading record">
              <Skeleton className="h-4 w-3/4 motion-reduce:animate-none" />
              <Skeleton className="h-4 w-full motion-reduce:animate-none" />
              <Skeleton className="h-4 w-1/2 motion-reduce:animate-none" />
            </div>
          )}
          {detail.data && detail.data.references.length > 0 && (
            <section className="mt-8">
              <h3 className="mb-3 flex items-center gap-2 text-sm font-medium">
                References
                <Badge variant="secondary" className="rounded-sm tabular-nums">
                  {detail.data.references.length}
                </Badge>
              </h3>
              <div className="space-y-2">
                {detail.data.references.map((target) => (
                  <div className="rounded-md border px-3 py-2.5 text-sm" key={target}>
                    <RecordLink target={target} navigation={navigation} />
                  </div>
                ))}
              </div>
            </section>
          )}
        </div>
        <div className="mt-auto border-t p-6">
          <p className="mb-4 text-xs leading-5 text-muted-foreground">
            Studio is read-only. Records and index definitions are managed by your application.
          </p>
          <Button variant="outline" className="w-full rounded-md" onClick={close}>
            Close Inspector
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  )
}
